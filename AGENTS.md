# Agent Guide — `webtyp/agentcontext`

Constraints for agents working on this library. **Read this before any change.**
The current work order, when one exists, is [docs/PLAN.md](docs/PLAN.md). The ecosystem plan
is [`agent/docs/AGENT_ECOSYSTEM_MASTER_PLAN.md`](https://github.com/webtyp/agent/blob/main/docs/AGENT_ECOSYSTEM_MASTER_PLAN.md).

---

## What this library is

The context compiler of `webtyp/agent`. **Every exported function is pure.** It receives
values and returns values. It never:

- reads or writes memory (that is the orchestrator's memory store),
- calls a model (the orchestrator calls `llm.Client`),
- reads the clock or generates IDs (timestamps and IDs arrive in the input).

If a change needs any of those, it belongs in `webtyp/agent`, not here.

It must never import `webtyp/agent`, because `agent` imports this library.

---

## The builds that define "done"

```bash
go vet ./...
gotest
gotest -tinygo
GOOS=js GOARCH=wasm go build ./...
```

---

## Never import these

| Never | Use instead | Why |
|---|---|---|
| `fmt`, `errors`, `strings`, `strconv` | `webtyp.com/fmt` | isomorphic, small under TinyGo |
| `sort` | the insertion sort in this repo | `sort.Slice` uses reflection, a size tax under TinyGo |
| `context` (stdlib) | `webtyp.com/context` | only if a function ever needs one (none does today) |
| `time` (stdlib), `time.Now` | `webtyp.com/time`'s pure UTC formatters only (`FormatISO8601`, `Weekday`) | timestamps and the users' offset arrive in the input; this library never reads a clock |
| `encoding/json` | nothing | reflection JSON costs ~1 MB of wasm |
| `map[K]V` | a slice | TinyGo's map runtime is a size tax |

---

## Common mistakes to avoid

- Truncating a request that does not fit. `Compile` returns an error, and dropping messages
  silently is exactly what it exists to prevent.
- Moving a changing value (date, retrieved chunks) into `System`. `System` must stay
  identical across turns so the runtime can cache it. Changing content goes in `Messages`.
- Mutating the caller's slices while sorting. Sort a copy.
- Counting tokens with `len(s)/4` in library code. Only `llm.TokenCounter` counts.
