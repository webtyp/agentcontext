---
PLAN: "feat: agentcontext — pure context compiler (Compile, Compact, SummaryRequest)"
TAG: v0.1.0
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
>
> **Phase 2** of
> [`AGENT_ECOSYSTEM_MASTER_PLAN.md`](https://github.com/webtyp/agent/blob/main/docs/AGENT_ECOSYSTEM_MASTER_PLAN.md).
> **Blocked until `webtyp.com/llm` v0.1.0 is published** (phase 1). `webtyp/agent` (phase 3)
> waits for this tag.

# Plan — `webtyp.com/agentcontext`: build a small, budgeted request from the agent's state

## 0. Context

Before each call to the model, `webtyp/agent` must decide **what the model gets to see**: the
agent's identity, a summary of the old conversation, the recent messages, and the tools on
offer, all within the model's context window. Today that logic lives in
`agent/context_window.go` (155 lines), mixed with three other jobs: reading memory, writing
memory, and calling the model to summarize.

This repository takes the **decision** part only and makes it pure: functions that receive
values and return values, with no I/O, no model call and no clock. The orchestrator keeps the
I/O. The result can be tested with plain data, and it is the single place where the context
engineering rules live (see `docs/CONTEXT_ENGINEERING.md` in this repo).

Three defects of the current code are fixed on the way. The tests below pin each one:

1. **Summaries come out in the wrong order.** `context_window.go` assumes the store returns
   summaries newest-first, but `webtyp/agentmemory` returns them oldest-first, so the prompt
   lists them backwards. The compiler now sorts by `CreatedAt` itself.
2. **The context size was sent as the output limit** (`MaxTokens: ContextWindow.MaxTokens`).
   `Budget` now has `ContextTokens` and `OutputTokens`, and only `OutputTokens` reaches
   `llm.Request.MaxOutputTokens`.
3. **Compaction can separate a tool result from the call that produced it.** When the cut
   falls between an assistant message with `ToolCalls` and its `RoleTool` answers, chat
   templates reject the request. The cut now moves past tool results.

## Development rules (inline)

- **Pure library.** No function in this repository performs I/O, calls a model, reads the
  clock or generates IDs. Everything it needs is a parameter.
- **Every file compiles for the browser** under `GOOS=js GOARCH=wasm` and TinyGo.
- **Never import:**

| Never | Use instead | Why |
|---|---|---|
| `context` (stdlib) | `webtyp.com/context` (not needed here: nothing does I/O) | |
| `fmt`, `errors`, `strings`, `strconv` | `webtyp.com/fmt` | isomorphic, small under TinyGo |
| `sort` | the insertion sort written in Stage 2 | `sort.Slice` uses reflection, which is a size tax under TinyGo |
| `encoding/json` | nothing | reflection JSON costs ~1 MB of wasm |
| `time` | nothing (timestamps arrive in `CreatedAt`) | |
| `map[K]V` | a slice | TinyGo's map runtime is a size tax |

- **No hardcoded strings in logic:** the prompt fragments are named constants (Stage 2).
- Flat layout, max 500 lines per file. Tests use `testing` only. Do **not** run `gopush`/`codejob`.
- Dependencies: `webtyp.com/llm` (v0.1.0) and `webtyp.com/fmt`. Nothing else.

## Design gate (api-design — five answers)

1. **Prior art.**
   - **Google ADK**: the *working context* is rebuilt for every invocation from session,
     memory and artifacts by a pipeline of processors, not kept as a growing transcript.
   - **LangChain `trim_messages` / `ConversationSummaryBufferMemory`**: token-budgeted
     trimming plus an LLM-written summary of what was cut.
   - **OpenAI Responses "compaction"** and **Anthropic context editing**: server-side
     compaction when the conversation nears the window, keeping a stable prefix cacheable.

   The difference here is that compilation is **pure** and the I/O stays in the caller. ADK and
   LangChain couple it to their memory objects. We cannot, because the memory backend is
   injected (`webtyp/agentmemory`, IndexedDB in the browser) and the model runtime is injected
   too.
2. **Novice-name test.** `agentcontext.Compile(input, budget, counter)` means "compile the
   agent's context". `Compact` returns "the turns to fold into a summary".
   `SummaryRequest(turns, budget)` is "the request that asks a model to summarize these
   turns". `Turn` is one message of the conversation as stored. `Summary` is the text that
   replaced some turns. `Budget` is the token limits. There are no boolean parameters.
3. **Complexity ledger.**
   ```
   Concepts the developer must learn   +3 (Compact, Compile, Budget.Validate) / −3 (ContextWindowConfig, Episode, the unexported prepareContext contract)
   Files they must touch to do X       +0 / −0
   Lines at the call site              +6 / −0   (agent calls Compact → maybe summarize → Compile)
   Ways to do the same thing           +0 / −1   (context_window.go is deleted in phase 3)
   ```
   Forgetting to call `Compact` is not silent: `Compile` returns an over-budget error that
   says "call Compact first".
4. **Where it belongs.** "What does the model see this turn" is one concern, separate from
   orchestration (FSM, tools) and from storage. It depends on `llm` for `Request` and
   `TokenCounter`. It must not import `agent`: `agent` imports it. The types it needs as input
   (`Turn`, `Summary`, `Identity`) are declared here, which is why `agent`'s memory ports will
   use them.
5. **What it deletes.** In phase 3, `webtyp/agent` deletes `context_window.go`
   (`prepareContext`, `summarizeMessages`, `buildSystemPrompt`), `ContextWindowConfig`
   (including the unused `BufferTokens`), `IdentityConfig` and `Episode`. The 50%-compaction
   rule, the identity prompt and the summary prompt move here and nowhere else.

## Stage 1 — types

**`types.go`**

```go
package agentcontext

import "webtyp.com/llm"

// Turn is one message of the conversation as the agent stores it.
type Turn struct {
	ID        string
	Message   llm.Message
	Tokens    int   // tokens of Message.Content, counted with the model's TokenCounter when stored
	CreatedAt int64 // unix seconds; the compiler orders turns by this, oldest first
}

// Summary is text written by a model that replaces the turns FromTurnID..ToTurnID.
type Summary struct {
	ID         string
	Text       string
	Tokens     int // tokens of Text
	FromTurnID string
	ToTurnID   string
	CreatedAt  int64 // unix seconds; the compiler orders summaries by this, oldest first
}

// Identity is who the agent is. It becomes llm.Request.System, the part of every request
// that never changes during a conversation.
type Identity struct {
	Name         string
	Role         string
	Instructions string
	Goals        []string
}

// Budget is the token limits of the model that will read the request.
type Budget struct {
	ContextTokens int     // the model's context window, e.g. 8192. Required.
	OutputTokens  int     // reserved for the answer; becomes Request.MaxOutputTokens. Required.
	CompactAt     float64 // fraction of the input space that triggers compaction; 0 means DefaultCompactAt
}

// DefaultCompactAt is used when Budget.CompactAt is zero.
const DefaultCompactAt = 0.8

// Input is the state the compiler reads.
type Input struct {
	Identity  Identity
	Summaries []Summary
	Turns     []Turn
	Tools     []llm.ToolDef
}
```

## Stage 2 — the three functions

**`prompts.go`**: every prompt fragment as a named constant. The strings are copied verbatim
from today's `agent/context_window.go`:

```go
const (
	summaryHeader       = "Previous conversation summary:\n"
	summaryLinePrefix   = "- "
	summarizerSystem    = "You are a helpful assistant that summarizes conversations."
	summarizerInstruct  = "Summarize the following conversation segment concisely:\n\n"
	identityGoalsHeader = "Goals:\n"
)
```

**`compile.go`**:

```go
// Compile builds the request the model reads this turn. It never truncates: if the input does
// not fit the budget, it returns an error, and the caller must Compact first.
func Compile(in Input, b Budget, c llm.TokenCounter) (llm.Request, error)
```

Exact behaviour:
1. `if err := b.Validate(); err != nil { return llm.Request{}, err }`.
2. `system := renderIdentity(in.Identity)`, byte-for-byte the output of today's
   `buildSystemPrompt`: `"You are {Name}. {Role}\n"` + `Instructions + "\n"` + (if
   `len(Goals) > 0`) `identityGoalsHeader` + one `"- {goal}\n"` per goal.
3. Sort a **copy** of `in.Summaries` and a **copy** of `in.Turns` by `CreatedAt` ascending, using
   a stable insertion sort (`sortTurns`, `sortSummaries`). Never mutate the caller's slices.
4. `messages`: if there is at least one summary, first one `llm.Message{Role: llm.RoleSystem, Content: summaryHeader + "- " + text + "\n" for each summary, oldest first}`;
   then every turn's `Message`, oldest first.
5. `used := used(in, system, c)` (see below). If `used + b.OutputTokens > b.ContextTokens`,
   return `fmt.Errf("agentcontext: request needs %d tokens but the budget allows %d; call Compact first", used+b.OutputTokens, b.ContextTokens)`.
6. Return `llm.Request{System: system, Messages: messages, Tools: in.Tools, MaxOutputTokens: b.OutputTokens}`.

```go
// Compact returns the oldest turns that must be folded into a Summary before Compile fits,
// oldest first, or nil when the input is within budget.
func Compact(in Input, b Budget, c llm.TokenCounter) []Turn
```

Exact behaviour:
1. If `b.Validate() != nil` or `len(in.Turns) < 2`, return nil.
2. Sort a copy of the turns. `limit := compactAt(b) × float64(b.ContextTokens − b.OutputTokens)`.
3. If `float64(used(in, renderIdentity(in.Identity), c)) <= limit`, return nil.
4. `cut := len(turns)/2`, minimum 1. Then, **while `cut < len(turns)` and `turns[cut].Message.Role == llm.RoleTool`, `cut++`**.
   If `cut == len(turns)`, return nil. Everything would be folded, and there would be
   nothing left to answer.
5. Return `turns[:cut]`.

```go
// SummaryRequest is the request that asks a model to summarize turns into one Summary.
func SummaryRequest(turns []Turn, b Budget) llm.Request
```

Returns `llm.Request{System: summarizerSystem, Messages: []llm.Message{{Role: llm.RoleUser, Content: body}}, MaxOutputTokens: b.OutputTokens}`,
where `body` is `summarizerInstruct` followed by, for each turn in the given order,
`"{role}: {content}\n"` and, for each of its tool calls, `"Tool Call {id}: {name}({input})\n"`.
This is the same text today's `summarizeMessages` builds.

Exported method (in `types.go`, next to `Budget`). `webtyp/agent` calls it in `New`, so an invalid budget fails at construction and not on the first turn:

```go
// Validate reports why b cannot budget a request, or nil.
func (b Budget) Validate() error
```

- `Budget.Validate()`: `ContextTokens <= 0` → `fmt.Err("agentcontext: Budget.ContextTokens must be greater than zero")`;
  `OutputTokens <= 0` → `fmt.Err("agentcontext: Budget.OutputTokens must be greater than zero")`;
  `OutputTokens >= ContextTokens` → `fmt.Err("agentcontext: Budget.OutputTokens must be smaller than Budget.ContextTokens")`.

Unexported helpers (`compile.go`):
- `compactAt(b Budget) float64`: `b.CompactAt`, or `DefaultCompactAt` when zero.
- `used(in Input, system string, c llm.TokenCounter) int`: `c.CountTokens(system)` + for each tool
  `c.CountTokens(Name) + c.CountTokens(Description) + c.CountTokens(InputSchema)` + (if any summary)
  `c.CountTokens(summaryHeader)` + Σ `summary.Tokens` + Σ `turn.Tokens`.

## Stage 3 — tests (package `agentcontext_test`)

**File:** `compile_test.go`. In the test file only, declare
`type quarterCounter struct{}` with `CountTokens(s string) int { return len(s) / 4 }`.

| Test | Asserts |
|---|---|
| `TestCompile_IdentityIsSystem` | `Identity{Name:"Ana", Role:"receptionist", Instructions:"Be brief.", Goals:[]string{"book"}}` → `System == "You are Ana. receptionist\nBe brief.\nGoals:\n- book\n"` |
| `TestCompile_SummariesOldestFirst` | summaries given newest-first (CreatedAt 20, 10) → the first message is `RoleSystem` with `"a"` before `"b"` (defect 1) |
| `TestCompile_TurnsOldestFirstAndCallerSliceUntouched` | turns out of order → request ordered; the input slice is unchanged afterwards |
| `TestCompile_OutputLimitIsOutputTokens` | `Budget{ContextTokens: 8192, OutputTokens: 512}` → `MaxOutputTokens == 512` (defect 2) |
| `TestCompile_OverBudgetErrors` | a turn with `Tokens: 9000`, context 8192 → error containing `call Compact first` |
| `TestCompile_InvalidBudget` | zero `ContextTokens`, zero `OutputTokens`, `OutputTokens >= ContextTokens`: each returns its exact message |
| `TestCompact_WithinBudgetReturnsNil` | small input → nil |
| `TestCompact_FoldsOldestHalf` | 10 turns of 100 tokens, context 1000, output 100 → returns the 5 oldest |
| `TestCompact_NeverSplitsToolResult` | turns: user, assistant(with ToolCalls), tool, tool, user, assistant, all over budget → half is 3, turn[3] is `RoleTool` → cut moves to 4 (defect 3) |
| `TestCompact_SingleTurnReturnsNil` | one huge turn → nil (nothing to fold into) |
| `TestSummaryRequest_Body` | exact `Messages[0].Content` for one user turn and one assistant turn with a tool call |

**Consumer-shaped test** (`flow_test.go`): it runs the loop the orchestrator will run, with a
scripted fake `llm.Client` in the test file. Start with 10 over-budget turns. Call `Compact`,
send `SummaryRequest` to the fake, and build a `Summary` from `resp.Text` and
`resp.Usage.OutputTokens`. Remove the folded turns, then call `Compile`. Assert that the
request fits (no error) and that its first message contains the fake's summary text.

## Stage 4 — README

Update `README.md`. Remove the `STATUS` note, and check that the "I want X → use Y" table
matches the signatures above exactly.

## Stages

| Stage | Files | Acceptance |
|---|---|---|
| 1 | `types.go`, `go.mod` | builds for host, wasm, TinyGo |
| 2 | `prompts.go`, `compile.go` | `grep -rn '"sort"\|"strings"\|"time"\|map\[' --include=*.go .` → empty |
| 3 | `compile_test.go`, `flow_test.go` | all tests pass under `gotest` and `gotest -tinygo` |
| 4 | `README.md` | no `STATUS` line remains |
