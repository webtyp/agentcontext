package agentcontext

import (
	"webtyp.com/fmt"
	"webtyp.com/llm"
)

// Compile builds the request the model reads this turn. It never truncates: if the input does
// not fit the budget, it returns an error, and the caller must Compact first.
func Compile(in Input, b Budget, c llm.TokenCounter) (llm.Request, error) {
	if err := b.Validate(); err != nil {
		return llm.Request{}, err
	}

	system := renderIdentity(in.Identity)

	sortedSummaries := sortSummaries(in.Summaries)
	sortedTurns := sortTurns(in.Turns)

	var messages []llm.Message
	if len(sortedSummaries) > 0 {
		var summaryContent string
		summaryContent += summaryHeader
		for i := 0; i < len(sortedSummaries); i++ {
			summaryContent += summaryLinePrefix + sortedSummaries[i].Text + "\n"
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: summaryContent,
		})
	}

	for i := 0; i < len(sortedTurns); i++ {
		msg := sortedTurns[i].Message
		if msg.Role == llm.RoleUser {
			msg.Content = Stamp(sortedTurns[i].CreatedAt, in.UTCOffsetMinutes) + msg.Content
		}
		messages = append(messages, msg)
	}

	totalUsed := used(in, system, c)
	if totalUsed+b.OutputTokens > b.ContextTokens {
		return llm.Request{}, fmt.Errf("agentcontext: request needs %d tokens but the budget allows %d; call Compact first", totalUsed+b.OutputTokens, b.ContextTokens)
	}

	return llm.Request{
		System:          system,
		Messages:        messages,
		Tools:           in.Tools,
		MaxOutputTokens: b.OutputTokens,
	}, nil
}

// Compact returns the oldest turns that must be folded into a Summary before Compile fits,
// oldest first, or nil when the input is within budget.
func Compact(in Input, b Budget, c llm.TokenCounter) []Turn {
	if b.Validate() != nil || len(in.Turns) < 2 {
		return nil
	}

	sortedTurns := sortTurns(in.Turns)
	system := renderIdentity(in.Identity)

	limit := compactAt(b) * float64(b.ContextTokens-b.OutputTokens)
	if float64(used(in, system, c)) <= limit {
		return nil
	}

	cut := len(sortedTurns) / 2
	if cut < 1 {
		cut = 1
	}

	for cut < len(sortedTurns) && sortedTurns[cut].Message.Role == llm.RoleTool {
		cut++
	}

	if cut == len(sortedTurns) {
		return nil
	}

	return sortedTurns[:cut]
}

// SummaryRequest is the request that asks a model to summarize turns into one Summary.
func SummaryRequest(turns []Turn, b Budget) llm.Request {
	var body string
	body += summarizerInstruct

	for i := 0; i < len(turns); i++ {
		t := turns[i]
		body += fmt.Sprintf("%s: %s\n", t.Message.Role, t.Message.Content)
		for j := 0; j < len(t.Message.ToolCalls); j++ {
			tc := t.Message.ToolCalls[j]
			body += fmt.Sprintf("Tool Call %s: %s(%s)\n", tc.ID, tc.Name, tc.Input)
		}
	}

	return llm.Request{
		System: summarizerSystem,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: body,
			},
		},
		MaxOutputTokens: b.OutputTokens,
	}
}

func renderIdentity(id Identity) string {
	var system string
	system += fmt.Sprintf("You are %s. %s\n", id.Name, id.Role)
	system += id.Instructions + "\n"
	if len(id.Goals) > 0 {
		system += identityGoalsHeader
		for i := 0; i < len(id.Goals); i++ {
			system += fmt.Sprintf("- %s\n", id.Goals[i])
		}
	}
	return system
}

func compactAt(b Budget) float64 {
	if b.CompactAt == 0 {
		return DefaultCompactAt
	}
	return b.CompactAt
}

func used(in Input, system string, c llm.TokenCounter) int {
	tokens := c.CountTokens(system)

	for i := 0; i < len(in.Tools); i++ {
		tool := in.Tools[i]
		tokens += c.CountTokens(tool.Name) + c.CountTokens(tool.Description) + c.CountTokens(tool.InputSchema)
	}

	if len(in.Summaries) > 0 {
		tokens += c.CountTokens(summaryHeader)
		for i := 0; i < len(in.Summaries); i++ {
			tokens += in.Summaries[i].Tokens
		}
	}

	for i := 0; i < len(in.Turns); i++ {
		tokens += in.Turns[i].Tokens
		if in.Turns[i].Message.Role == llm.RoleUser {
			tokens += c.CountTokens(Stamp(in.Turns[i].CreatedAt, in.UTCOffsetMinutes))
		}
	}

	return tokens
}

// sortTurns performs a stable insertion sort on a copy of turns by CreatedAt ascending.
func sortTurns(turns []Turn) []Turn {
	if len(turns) == 0 {
		return nil
	}
	dst := make([]Turn, len(turns))
	copy(dst, turns)

	for i := 1; i < len(dst); i++ {
		key := dst[i]
		j := i - 1
		for j >= 0 && dst[j].CreatedAt > key.CreatedAt {
			dst[j+1] = dst[j]
			j--
		}
		dst[j+1] = key
	}
	return dst
}

// sortSummaries performs a stable insertion sort on a copy of summaries by CreatedAt ascending.
func sortSummaries(summaries []Summary) []Summary {
	if len(summaries) == 0 {
		return nil
	}
	dst := make([]Summary, len(summaries))
	copy(dst, summaries)

	for i := 1; i < len(dst); i++ {
		key := dst[i]
		j := i - 1
		for j >= 0 && dst[j].CreatedAt > key.CreatedAt {
			dst[j+1] = dst[j]
			j--
		}
		dst[j+1] = key
	}
	return dst
}
