package memory

import (
	"fmt"
	"strings"
)

// DefaultPromptBuilder implements PromptBuilder. It constructs a system
// prompt enriched with relevant memories while respecting a token budget.
type DefaultPromptBuilder struct{}

// NewPromptBuilder creates a prompt builder.
func NewPromptBuilder() *DefaultPromptBuilder {
	return &DefaultPromptBuilder{}
}

// MemoryBuilder (meory_builder) formats the raw memories into a structured array of strings.
type MemoryBuilder struct{}

func (mb *MemoryBuilder) Build(memories []*SearchResult, maxTokens int) ([]string, int) {
	var lines []string
	tokensUsed := 0
	for _, sr := range memories {
		line := formatMemoryLine(sr.Memory, sr.Score)
		lineTokens := estimateTokens(line)
		if tokensUsed+lineTokens > maxTokens {
			break
		}
		lines = append(lines, line)
		tokensUsed += lineTokens
	}
	return lines, tokensUsed
}

// ContextBuilder formats the memory list into the XML context block.
type ContextBuilder struct{}

func (cb *ContextBuilder) Build(lines []string) string {
	var sb strings.Builder
	if len(lines) > 0 {
		sb.WriteString("\n\n<memory_context>\n")
		for _, line := range lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		sb.WriteString("</memory_context>\n\n")
		sb.WriteString("Use the above memories to personalize your responses. ")
		sb.WriteString("Reference relevant memories naturally without explicitly mentioning ")
		sb.WriteString("that you have a memory system unless the user asks about it.\n")
	}

	sb.WriteString("\n<operational_profile_guidelines>\n")
	sb.WriteString("- If a user request lacks key information or context, explicitly ask the user for clarification rather than assuming or guessing.\n")
	sb.WriteString("- Use the `check_system_resources` tool to verify RAM allocation and CPU resources when executing resource-heavy operations or when uncertain.\n")
	sb.WriteString("</operational_profile_guidelines>\n\n")

	return sb.String()
}

// ModelFinalPrompt combines the original system prompt with the memory context block.
func ModelFinalPrompt(systemPrompt string, contextBlock string) string {
	return systemPrompt + contextBlock
}

// Build injects ranked memories into the system prompt. It stays within
// maxTokens by estimating token counts and truncating when necessary.
func (b *DefaultPromptBuilder) Build(memories []*SearchResult, systemPrompt string, maxTokens int) string {
	if len(memories) == 0 {
		return systemPrompt
	}

	// Pipeline: [input] -> [memory(knnSearch)] -> [meory_builder] -> [contextBuilder] -> [ModelFinalPromt]
	mb := &MemoryBuilder{}
	lines, _ := mb.Build(memories, maxTokens)

	cb := &ContextBuilder{}
	contextBlock := cb.Build(lines)

	return ModelFinalPrompt(systemPrompt, contextBlock)
}

// formatMemoryLine renders a single memory for prompt injection with source citations.
func formatMemoryLine(mem *Memory, score float64) string {
	tags := ""
	if len(mem.Tags) > 0 {
		tags = " [" + strings.Join(mem.Tags, ", ") + "]"
	}

	content := mem.Content
	if mem.Summary != "" {
		content = mem.Summary
	}

	var citationParts []string
	if mem.Source != "" {
		citationParts = append(citationParts, "Source: "+mem.Source)
	}
	if mem.PageNumber > 0 {
		citationParts = append(citationParts, fmt.Sprintf("Page: %d", mem.PageNumber))
	}
	if mem.StartLine > 0 && mem.EndLine > 0 {
		citationParts = append(citationParts, fmt.Sprintf("Lines: %d-%d", mem.StartLine, mem.EndLine))
	}

	citation := ""
	if len(citationParts) > 0 {
		citation = fmt.Sprintf(" (%s)", strings.Join(citationParts, " | "))
	}

	return fmt.Sprintf("- [%s|%.0f%%] %s%s%s",
		mem.Type,
		score*100,
		content,
		tags,
		citation,
	)
}

// estimateTokens approximates token count. Uses the ~4 chars/token heuristic
// which is a reasonable average across English text with GPT-style tokenisers.
func estimateTokens(s string) int {
	n := len(s) / 4
	if n == 0 && len(s) > 0 {
		n = 1
	}
	return n
}
