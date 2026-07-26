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

// Build injects ranked memories into the system prompt. It stays within
// maxTokens by estimating token counts and truncating when necessary.
func (b *DefaultPromptBuilder) Build(memories []*SearchResult, systemPrompt string, maxTokens int) string {
	if len(memories) == 0 {
		return systemPrompt
	}

	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString("\n\n<memory_context>\n")

	tokensUsed := 0
	memCount := 0

	for _, sr := range memories {
		mem := sr.Memory
		line := formatMemoryLine(mem, sr.Score)
		lineTokens := estimateTokens(line)

		if tokensUsed+lineTokens > maxTokens {
			break
		}

		sb.WriteString(line)
		sb.WriteByte('\n')
		tokensUsed += lineTokens
		memCount++
	}

	if memCount == 0 {
		// No memories fit within budget — return original prompt
		return systemPrompt
	}

	sb.WriteString("</memory_context>\n\n")
	sb.WriteString("Use the above memories to personalize your responses. ")
	sb.WriteString("Reference relevant memories naturally without explicitly mentioning ")
	sb.WriteString("that you have a memory system unless the user asks about it.")

	return sb.String()
}

// formatMemoryLine renders a single memory for prompt injection.
func formatMemoryLine(mem *Memory, score float64) string {
	tags := ""
	if len(mem.Tags) > 0 {
		tags = " [" + strings.Join(mem.Tags, ", ") + "]"
	}

	content := mem.Content
	if mem.Summary != "" {
		content = mem.Summary
	}

	return fmt.Sprintf("- [%s|%.0f%%] %s%s",
		mem.Type,
		score*100,
		content,
		tags,
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
