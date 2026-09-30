package agentcontext

import (
	"webtyp.com/fmt"
	"webtyp.com/llm"
)

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

	// UTCOffsetMinutes is the users' timezone, e.g. -180 for UTC-3. Every user turn reaches the
	// model prefixed with the local date and time it was said, so the model knows what "today"
	// and "now" are.
	UTCOffsetMinutes int
}

// Validate reports why b cannot budget a request, or nil.
func (b Budget) Validate() error {
	if b.ContextTokens <= 0 {
		return fmt.Err("agentcontext: Budget.ContextTokens must be greater than zero")
	}
	if b.OutputTokens <= 0 {
		return fmt.Err("agentcontext: Budget.OutputTokens must be greater than zero")
	}
	if b.OutputTokens >= b.ContextTokens {
		return fmt.Err("agentcontext: Budget.OutputTokens must be smaller than Budget.ContextTokens")
	}
	return nil
}
