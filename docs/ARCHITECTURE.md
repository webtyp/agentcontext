# Architecture — `webtyp/agentcontext`

## What this is

A **context compiler**: a library that turns the agent's state into the one request the
model reads on this turn. "Context" here means everything inside the model's context
window: the instructions, the conversation, and the tool descriptions.

You meet it inside `webtyp/agent`. On every reasoning turn, the orchestrator loads recent
turns and summaries from memory, asks `agentcontext` what to send, and sends it.

It exists because a small model running in the browser has a small window and a slow CPU.
Every token counts twice, once for fitting and once for the time the model spends reading it.
The rules that keep the request small need one home that can be tested without a model or a
database. The reasoning behind those rules is in
[CONTEXT_ENGINEERING.md](CONTEXT_ENGINEERING.md).

## The boundary: decisions here, I/O in the orchestrator

| This library (pure) | The orchestrator, `webtyp/agent` (I/O) |
|---|---|
| decides which turns must be summarized (`Compact`) | loads turns and summaries from its memory store |
| writes the summarization prompt (`SummaryRequest`) | calls the summarizer model |
| builds the final request (`Compile`) | saves the `Summary`, deletes the folded turns |
| renders the identity into `System` | assigns IDs and timestamps |

Keeping I/O out makes three things possible. The whole policy is testable with plain
values. The same compiler works with any memory backend (IndexedDB in the browser, SQL on a
server). And the orchestrator cannot drift into a second copy of the rules.

## Contracts it depends on

- `webtyp/llm`: `llm.Request` is the output, `llm.TokenCounter` counts with the model's own
  tokenizer, and `llm.Message` is the payload of each `Turn`.
- Nothing else. It must not import `agent`, because `agent` imports it.

`Turn`, `Summary` and `Identity` are declared **here** because the compiler reads them.
`agent` declares its memory ports in terms of these types, and `agentmemory` stores them.

[Context window diagram](diagrams/CONTEXT_WINDOW.md): one turn, step by step.

## What is implemented and what comes next

The principles of [CONTEXT_ENGINEERING.md](CONTEXT_ENGINEERING.md), mapped to where they stand:

| Principle | Where |
|---|---|
| State ≠ context: the request is rebuilt every turn from stored state | v0.1.0 (`Compile`) |
| Stable prefix: `System` only holds the identity, which never changes mid-conversation | v0.1.0 |
| Compaction: fold the oldest turns into a summary, never truncate silently | v0.1.0 (`Compact`, `SummaryRequest`) |
| Budget counted with the model's tokenizer | v0.1.0 (`llm.TokenCounter`) |
| Structured task state (goal, status, entities) as its own section | next |
| Retrieved knowledge as its own section, with its own budget | next, fed by `webtyp/retrieval` through the agent |
| Only 2 permanent tools (`search_tools`, `execute_tool`) and schemas loaded on demand | next, in `webtyp/agent` (see its architecture) |
| Per-section token budgets | next |

The "next" rows do not have a plan yet. Each gets its own `docs/PLAN.md` when it is taken on.
