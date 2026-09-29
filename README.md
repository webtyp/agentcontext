# agentcontext

The context compiler of `webtyp/agent`. Before each call to the model, it decides what the
model sees: who the agent is, a summary of the old conversation, the recent messages, and the
tools on offer, all within the model's token budget. It is pure. It never reads memory, never
calls a model and never reads the clock, so everything it needs is a parameter.

## Getting started

The orchestrator runs this on every turn:

```go
in := agentcontext.Input{Identity: id, Summaries: sums, Turns: turns, Tools: tools}

if old := agentcontext.Compact(in, budget, counter); old != nil {
	resp, err := summarizer.Generate(ctx, agentcontext.SummaryRequest(old, budget))
	// save a Summary built from resp, delete the old turns, rebuild `in`
}

req, err := agentcontext.Compile(in, budget, counter) // an llm.Request, or "call Compact first"
```

| I want to… | Use |
|---|---|
| build the request for this turn | `Compile(in, budget, counter)` |
| know which turns to summarize | `Compact(in, budget, counter)` |
| ask a model for that summary | `SummaryRequest(turns, budget)` |
| set the limits of my model | `Budget{ContextTokens, OutputTokens, CompactAt}` |

`counter` is an `llm.TokenCounter`, the tokenizer of the model that will read the request.

## Documentation

- [Architecture](docs/ARCHITECTURE.md): what the compiler does, what it leaves to the orchestrator, and why.
- [Context engineering](docs/CONTEXT_ENGINEERING.md): the principles this library applies (state ≠ context, stable prefix, compaction, progressive disclosure).
- [Context window diagram](docs/diagrams/CONTEXT_WINDOW.md): one turn, step by step.
- [Agent guide](AGENTS.md): rules for anyone changing this library.
