package agentcontext_test

import (
	"testing"

	"webtyp.com/agentcontext"
	"webtyp.com/llm"
)

// 2026-09-29 13:00:00 UTC, a Tuesday: 10:00 in Chile (UTC-3).
const tuesday1300UTC = 1790686800

func TestCompile_UserTurnsCarryLocalDateAndTime(t *testing.T) {
	in := agentcontext.Input{
		Turns: []agentcontext.Turn{
			{ID: "u", Message: llm.Message{Role: llm.RoleUser, Content: "¿Hasta qué hora atendemos hoy?"}, CreatedAt: tuesday1300UTC},
			{ID: "a", Message: llm.Message{Role: llm.RoleAssistant, Content: "Hasta las 18:00."}, CreatedAt: tuesday1300UTC + 5},
		},
		UTCOffsetMinutes: -180,
	}

	req, err := agentcontext.Compile(in, agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}, quarterCounter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "[2026-09-29 Tuesday 10:00 UTC-03:00]\n¿Hasta qué hora atendemos hoy?"
	if req.Messages[0].Content != want {
		t.Errorf("user turn:\n got %q\nwant %q", req.Messages[0].Content, want)
	}
	if req.Messages[1].Content != "Hasta las 18:00." {
		t.Errorf("assistant turns are not stamped, got %q", req.Messages[1].Content)
	}
	if in.Turns[0].Message.Content != "¿Hasta qué hora atendemos hoy?" {
		t.Errorf("the stored turn was modified: %q", in.Turns[0].Message.Content)
	}
}

func TestCompile_LocalDateCrossesMidnight(t *testing.T) {
	// 2026-09-30 01:30 UTC is still Tuesday 22:30 in UTC-3; with +05:30 it is Wednesday 07:00.
	const at = tuesday1300UTC + 12*3600 + 30*60
	cases := []struct {
		offset int
		want   string
	}{
		{-180, "[2026-09-29 Tuesday 22:30 UTC-03:00]\nhola"},
		{330, "[2026-09-30 Wednesday 07:00 UTC+05:30]\nhola"},
	}
	for _, c := range cases {
		in := agentcontext.Input{
			Turns:            []agentcontext.Turn{{ID: "u", Message: llm.Message{Role: llm.RoleUser, Content: "hola"}, CreatedAt: at}},
			UTCOffsetMinutes: c.offset,
		}
		req, err := agentcontext.Compile(in, agentcontext.Budget{ContextTokens: 8192, OutputTokens: 512}, quarterCounter{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Messages[0].Content != c.want {
			t.Errorf("offset %d:\n got %q\nwant %q", c.offset, req.Messages[0].Content, c.want)
		}
	}
}

func TestCompile_StampCountsAgainstTheBudget(t *testing.T) {
	// One user turn of 100 tokens. With quarterCounter the stamp
	// "[1970-01-01 Thursday 00:00 UTC+00:00]\n" adds 9 tokens and the empty identity
	// "You are . \n\n" adds 3, so with 12 of output the request needs exactly 124.
	in := agentcontext.Input{Turns: []agentcontext.Turn{
		{ID: "u", Message: llm.Message{Role: llm.RoleUser, Content: "x"}, Tokens: 100},
	}}
	if _, err := agentcontext.Compile(in, agentcontext.Budget{ContextTokens: 123, OutputTokens: 12}, quarterCounter{}); err == nil {
		t.Fatal("expected the stamp tokens to push the request over budget")
	}
	if _, err := agentcontext.Compile(in, agentcontext.Budget{ContextTokens: 124, OutputTokens: 12}, quarterCounter{}); err != nil {
		t.Fatalf("100 + 9 + 3 + 12 = 124 must fit: %v", err)
	}
}
