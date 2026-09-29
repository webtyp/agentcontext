package agentcontext_test

import (
	"testing"

	"webtyp.com/agentcontext"
	"webtyp.com/fmt"
	"webtyp.com/llm"
)

type quarterCounter struct{}

func (quarterCounter) CountTokens(s string) int {
	return len(s) / 4
}

func TestCompile_IdentityIsSystem(t *testing.T) {
	id := agentcontext.Identity{
		Name:         "Ana",
		Role:         "receptionist",
		Instructions: "Be brief.",
		Goals:        []string{"book"},
	}
	in := agentcontext.Input{Identity: id}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	req, err := agentcontext.Compile(in, b, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedSystem := "You are Ana. receptionist\nBe brief.\nGoals:\n- book\n"
	if req.System != expectedSystem {
		t.Errorf("got system %q, want %q", req.System, expectedSystem)
	}
}

func TestCompile_SummariesOldestFirst(t *testing.T) {
	sums := []agentcontext.Summary{
		{ID: "s2", Text: "b", CreatedAt: 20, Tokens: 10},
		{ID: "s1", Text: "a", CreatedAt: 10, Tokens: 10},
	}
	in := agentcontext.Input{Summaries: sums}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	req, err := agentcontext.Compile(in, b, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) == 0 {
		t.Fatalf("expected system message for summaries, got 0 messages")
	}

	firstMsg := req.Messages[0]
	if firstMsg.Role != llm.RoleSystem {
		t.Errorf("expected RoleSystem, got %v", firstMsg.Role)
	}

	expectedContent := "Previous conversation summary:\n- a\n- b\n"
	if firstMsg.Content != expectedContent {
		t.Errorf("got summary content %q, want %q", firstMsg.Content, expectedContent)
	}
}

func TestCompile_TurnsOldestFirstAndCallerSliceUntouched(t *testing.T) {
	turns := []agentcontext.Turn{
		{ID: "t2", Message: llm.Message{Role: llm.RoleUser, Content: "second"}, CreatedAt: 20, Tokens: 5},
		{ID: "t1", Message: llm.Message{Role: llm.RoleUser, Content: "first"}, CreatedAt: 10, Tokens: 5},
	}
	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	req, err := agentcontext.Compile(in, b, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}

	if req.Messages[0].Content != "first" || req.Messages[1].Content != "second" {
		t.Errorf("messages not ordered oldest first: got %v, %v", req.Messages[0].Content, req.Messages[1].Content)
	}

	// Verify original slice is unchanged
	if turns[0].ID != "t2" || turns[1].ID != "t1" {
		t.Errorf("caller slice was mutated: %#v", turns)
	}
}

func TestCompile_OutputLimitIsOutputTokens(t *testing.T) {
	in := agentcontext.Input{}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	req, err := agentcontext.Compile(in, b, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.MaxOutputTokens != 512 {
		t.Errorf("got MaxOutputTokens %d, want 512", req.MaxOutputTokens)
	}
}

func TestCompile_OverBudgetErrors(t *testing.T) {
	turns := []agentcontext.Turn{
		{ID: "t1", Message: llm.Message{Role: llm.RoleUser, Content: "huge"}, CreatedAt: 10, Tokens: 9000},
	}
	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	_, err := agentcontext.Compile(in, b, c)
	if err == nil {
		t.Fatalf("expected over-budget error, got nil")
	}

	expectedErrSub := "call Compact first"
	if !fmt.Contains(err.Error(), expectedErrSub) {
		t.Errorf("got error %q, expected substring %q", err.Error(), expectedErrSub)
	}
}

func TestCompile_InvalidBudget(t *testing.T) {
	tests := []struct {
		name    string
		b       agentcontext.Budget
		wantErr string
	}{
		{
			name:    "zero ContextTokens",
			b:       agentcontext.Budget{ContextTokens: 0, OutputTokens: 100},
			wantErr: "agentcontext: Budget.ContextTokens must be greater than zero",
		},
		{
			name:    "zero OutputTokens",
			b:       agentcontext.Budget{ContextTokens: 1000, OutputTokens: 0},
			wantErr: "agentcontext: Budget.OutputTokens must be greater than zero",
		},
		{
			name:    "OutputTokens >= ContextTokens",
			b:       agentcontext.Budget{ContextTokens: 1000, OutputTokens: 1000},
			wantErr: "agentcontext: Budget.OutputTokens must be smaller than Budget.ContextTokens",
		},
	}

	in := agentcontext.Input{}
	c := quarterCounter{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := agentcontext.Compile(in, tt.b, c)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("got error %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestCompact_WithinBudgetReturnsNil(t *testing.T) {
	turns := []agentcontext.Turn{
		{ID: "t1", Message: llm.Message{Role: llm.RoleUser, Content: "hi"}, CreatedAt: 10, Tokens: 10},
		{ID: "t2", Message: llm.Message{Role: llm.RoleAssistant, Content: "hello"}, CreatedAt: 20, Tokens: 10},
	}
	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	compacted := agentcontext.Compact(in, b, c)
	if compacted != nil {
		t.Errorf("expected nil compacted, got %v turns", len(compacted))
	}
}

func TestCompact_FoldsOldestHalf(t *testing.T) {
	// 10 turns of 100 tokens
	// context 1000, output 100.
	// limit = 0.8 * (1000 - 100) = 720.
	// System tokens: ~7 tokens for system ("You are . \n\n").
	// Total used: 7 + 1000 = 1007 tokens > 720.
	var turns []agentcontext.Turn
	for i := 1; i <= 10; i++ {
		turns = append(turns, agentcontext.Turn{
			ID:        fmt.Sprintf("t%d", i),
			Message:   llm.Message{Role: llm.RoleUser, Content: "msg"},
			CreatedAt: int64(i * 10),
			Tokens:    100,
		})
	}

	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 1000, OutputTokens: 100}
	c := quarterCounter{}

	compacted := agentcontext.Compact(in, b, c)
	if len(compacted) != 5 {
		t.Fatalf("expected 5 compacted turns, got %d", len(compacted))
	}

	for i := 0; i < 5; i++ {
		expectedID := fmt.Sprintf("t%d", i+1)
		if compacted[i].ID != expectedID {
			t.Errorf("at index %d, got ID %s, want %s", i, compacted[i].ID, expectedID)
		}
	}
}

func TestCompact_NeverSplitsToolResult(t *testing.T) {
	// 6 turns:
	// 0: user
	// 1: assistant (with ToolCalls)
	// 2: tool
	// 3: tool
	// 4: user
	// 5: assistant
	// cut would normally be len/2 = 3.
	// But turn 3 has RoleTool, so cut should advance to 4.
	turns := []agentcontext.Turn{
		{ID: "t0", Message: llm.Message{Role: llm.RoleUser, Content: "call tool"}, CreatedAt: 10, Tokens: 100},
		{ID: "t1", Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "t"}}}, CreatedAt: 20, Tokens: 100},
		{ID: "t2", Message: llm.Message{Role: llm.RoleTool, ToolCallID: "c1", Content: "res1"}, CreatedAt: 30, Tokens: 100},
		{ID: "t3", Message: llm.Message{Role: llm.RoleTool, ToolCallID: "c1", Content: "res2"}, CreatedAt: 40, Tokens: 100},
		{ID: "t4", Message: llm.Message{Role: llm.RoleUser, Content: "next"}, CreatedAt: 50, Tokens: 100},
		{ID: "t5", Message: llm.Message{Role: llm.RoleAssistant, Content: "done"}, CreatedAt: 60, Tokens: 100},
	}

	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 500, OutputTokens: 50}
	c := quarterCounter{}

	compacted := agentcontext.Compact(in, b, c)
	if len(compacted) != 4 {
		t.Fatalf("expected cut at 4, got %d turns compacted", len(compacted))
	}
}

func TestCompact_SingleTurnReturnsNil(t *testing.T) {
	turns := []agentcontext.Turn{
		{ID: "t1", Message: llm.Message{Role: llm.RoleUser, Content: "huge"}, CreatedAt: 10, Tokens: 9000},
	}
	in := agentcontext.Input{Turns: turns}
	b := agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}
	c := quarterCounter{}

	compacted := agentcontext.Compact(in, b, c)
	if compacted != nil {
		t.Errorf("expected nil for single over-budget turn, got %v", compacted)
	}
}

func TestSummaryRequest_Body(t *testing.T) {
	turns := []agentcontext.Turn{
		{
			ID: "t1",
			Message: llm.Message{
				Role:    llm.RoleUser,
				Content: "Hello",
			},
		},
		{
			ID: "t2",
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: "Working...",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "get_weather", Input: `{"city":"London"}`},
				},
			},
		},
	}

	b := agentcontext.Budget{ContextTokens: 1000, OutputTokens: 200}
	req := agentcontext.SummaryRequest(turns, b)

	if req.System != "You are a helpful assistant that summarizes conversations." {
		t.Errorf("unexpected system prompt: %q", req.System)
	}

	if req.MaxOutputTokens != 200 {
		t.Errorf("got MaxOutputTokens %d, want 200", req.MaxOutputTokens)
	}

	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 user message, got %d", len(req.Messages))
	}

	expectedContent := "Summarize the following conversation segment concisely:\n\n" +
		"user: Hello\n" +
		"assistant: Working...\n" +
		"Tool Call call_1: get_weather({\"city\":\"London\"})\n"

	if req.Messages[0].Content != expectedContent {
		t.Errorf("got content:\n%q\nwant:\n%q", req.Messages[0].Content, expectedContent)
	}
}
