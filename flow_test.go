package agentcontext_test

import (
	"testing"

	"webtyp.com/agentcontext"
	"webtyp.com/context"
	"webtyp.com/fmt"
	"webtyp.com/llm"
)

type fakeClient struct {
	summaryResponse string
	summaryTokens   int
}

func (f *fakeClient) Generate(ctx *context.Context, req llm.Request) (llm.Response, error) {
	return llm.Response{
		Text: f.summaryResponse,
		Usage: llm.Usage{
			OutputTokens: f.summaryTokens,
		},
	}, nil
}

func (f *fakeClient) Stream(ctx *context.Context, req llm.Request) (llm.Streamer, error) {
	return nil, nil
}

func TestOrchestratorFlow(t *testing.T) {
	// Start with 10 over-budget turns
	var turns []agentcontext.Turn
	for i := 1; i <= 10; i++ {
		turns = append(turns, agentcontext.Turn{
			ID:        fmt.Sprintf("t%d", i),
			Message:   llm.Message{Role: llm.RoleUser, Content: "turn content"},
			CreatedAt: int64(i * 10),
			Tokens:    100,
		})
	}

	id := agentcontext.Identity{
		Name: "Assistant",
		Role: "Helper",
	}

	budget := agentcontext.Budget{
		ContextTokens: 1000,
		OutputTokens:  100,
	}
	counter := quarterCounter{}

	in := agentcontext.Input{
		Identity: id,
		Turns:    turns,
	}

	// First compile check should fail because 10 turns * 100 tokens + system overhead > 900
	_, err := agentcontext.Compile(in, budget, counter)
	if err == nil {
		t.Fatalf("expected Compile to fail before compaction")
	}

	// Orchestrator loop
	fake := &fakeClient{
		summaryResponse: "User had a 5-turn conversation.",
		summaryTokens:   20,
	}

	oldTurns := agentcontext.Compact(in, budget, counter)
	if oldTurns == nil {
		t.Fatalf("expected Compact to return old turns")
	}

	sumReq := agentcontext.SummaryRequest(oldTurns, budget)
	resp, err := fake.Generate(context.Background(), sumReq)
	if err != nil {
		t.Fatalf("summary generation failed: %v", err)
	}

	// Create summary and update turns
	newSummary := agentcontext.Summary{
		ID:         "s1",
		Text:       resp.Text,
		Tokens:     resp.Usage.OutputTokens,
		FromTurnID: oldTurns[0].ID,
		ToTurnID:   oldTurns[len(oldTurns)-1].ID,
		CreatedAt:  100,
	}

	// Remove folded turns
	remainingTurns := turns[len(oldTurns):]

	in.Summaries = append(in.Summaries, newSummary)
	in.Turns = remainingTurns

	// Now compile should succeed
	compiledReq, err := agentcontext.Compile(in, budget, counter)
	if err != nil {
		t.Fatalf("Compile failed after compaction: %v", err)
	}

	if len(compiledReq.Messages) < 1 {
		t.Fatalf("expected compiled messages")
	}

	firstMsg := compiledReq.Messages[0]
	if firstMsg.Role != llm.RoleSystem {
		t.Errorf("expected first message to be system summary message, got %v", firstMsg.Role)
	}

	if !fmt.Contains(firstMsg.Content, "User had a 5-turn conversation.") {
		t.Errorf("expected summary content in system message, got %q", firstMsg.Content)
	}
}
