package memory

import (
	"context"
	"regexp"
	"strings"
)

const minContentLength = 10

// PatternExtractor implements MemoryExtractor using rule-based pattern
// matching. It detects user facts, project info, episodic events, and
// preferences without requiring a second model call.
type PatternExtractor struct {
	userFactPatterns  []*regexp.Regexp
	projectPatterns   []*regexp.Regexp
	episodicPatterns  []*regexp.Regexp
	preferencePatterns []*regexp.Regexp
	noisePatterns     []*regexp.Regexp
}

// NewPatternExtractor compiles the extraction patterns.
func NewPatternExtractor() *PatternExtractor {
	return &PatternExtractor{
		userFactPatterns: compileAll(
			`(?i)\bmy name is\s+(\w+)`,
			`(?i)\bi am\s+(a\s+)?\w+`,
			`(?i)\bi work\s+(at|for|as|in)\s+`,
			`(?i)\bi live\s+(in|at|near)\s+`,
			`(?i)\bi speak\s+`,
			`(?i)\bi('m| am)\s+\d+\s+years?\s+old`,
			`(?i)\bi (have|got)\s+(a\s+)?(dog|cat|pet|kid|child|car)`,
			`(?i)\bi (study|studied|graduated)\s+`,
		),
		projectPatterns: compileAll(
			`(?i)\b(working on|building|developing|creating)\s+(a\s+)?`,
			`(?i)\b(the project|our project|this project|the codebase)\b`,
			`(?i)\b(repository|repo)\s+(is|at|on)\s+`,
			`(?i)\b(tech stack|using|framework)\s+(is|includes)\s+`,
			`(?i)\b(api|endpoint|service)\s+(at|is|for)\s+`,
		),
		episodicPatterns: compileAll(
			`(?i)\b(completed|finished|done with|deployed|launched|shipped)\s+`,
			`(?i)\b(created|built|implemented|fixed|resolved|merged)\s+`,
			`(?i)\b(error|bug|issue|problem)\s+(was|is|has been)\s+(fixed|resolved)`,
			`(?i)\b(milestone|release|version)\s+`,
			`(?i)\bsuccessfully\s+`,
		),
		preferencePatterns: compileAll(
			`(?i)\bi (prefer|like|love|enjoy|favor|hate|dislike)\s+`,
			`(?i)\bi (always|never|usually|typically)\s+(use|do|prefer|avoid)\s+`,
			`(?i)\b(my favorite|my preferred)\s+`,
			`(?i)\b(don't|do not)\s+(like|want|use|prefer)\s+`,
		),
		noisePatterns: compileAll(
			`(?i)^(hi|hello|hey|thanks|thank you|ok|okay|sure|yes|no|bye|goodbye)[\s!.]*$`,
			`(?i)^(how are you|what's up|good morning|good night)[\s!?.]*$`,
			`(?i)^(please|sorry|excuse me)[\s!.]*$`,
		),
	}
}

// Extract analyses a user–assistant message pair and returns candidate
// memories worth persisting.
func (e *PatternExtractor) Extract(_ context.Context, userMsg, assistantMsg string) ([]ExtractedMemory, error) {
	var results []ExtractedMemory

	// Analyse user message for facts and preferences
	if len(userMsg) >= minContentLength && !e.isNoise(userMsg) {
		results = append(results, e.extractFromText(userMsg, true)...)
	}

	// Analyse assistant message for episodic events and project info
	if len(assistantMsg) >= minContentLength && !e.isNoise(assistantMsg) {
		results = append(results, e.extractFromText(assistantMsg, false)...)
	}

	return dedup(results), nil
}

func (e *PatternExtractor) extractFromText(text string, isUser bool) []ExtractedMemory {
	var results []ExtractedMemory

	// Split long text into sentences for better extraction
	sentences := splitSentences(text)

	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < minContentLength {
			continue
		}

		if isUser {
			if matchesAny(e.userFactPatterns, sentence) {
				results = append(results, ExtractedMemory{
					Content:    sentence,
					Type:       MemoryTypeUser,
					Importance: 0.8,
					Tags:       extractTags(sentence),
				})
			}
			if matchesAny(e.preferencePatterns, sentence) {
				results = append(results, ExtractedMemory{
					Content:    sentence,
					Type:       MemoryTypeUser,
					Importance: 0.6,
					Tags:       append(extractTags(sentence), "preference"),
				})
			}
		}

		if matchesAny(e.projectPatterns, sentence) {
			results = append(results, ExtractedMemory{
				Content:    sentence,
				Type:       MemoryTypeProject,
				Importance: 0.7,
				Tags:       append(extractTags(sentence), "project"),
			})
		}

		if matchesAny(e.episodicPatterns, sentence) {
			results = append(results, ExtractedMemory{
				Content:    sentence,
				Type:       MemoryTypeEpisodic,
				Importance: 0.5,
				Tags:       append(extractTags(sentence), "event"),
			})
		}
	}

	return results
}

func (e *PatternExtractor) isNoise(text string) bool {
	return matchesAny(e.noisePatterns, strings.TrimSpace(text))
}

// -- helpers --

func compileAll(patterns ...string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		compiled[i] = regexp.MustCompile(p)
	}
	return compiled
}

func matchesAny(patterns []*regexp.Regexp, text string) bool {
	for _, p := range patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func splitSentences(text string) []string {
	// Simple sentence splitting on period, exclamation, question mark
	// followed by space or end of string
	re := regexp.MustCompile(`[.!?]+\s+`)
	return re.Split(text, -1)
}

func extractTags(text string) []string {
	// Extract notable keywords as tags
	tagPatterns := map[string]*regexp.Regexp{
		"code":     regexp.MustCompile(`(?i)\b(code|programming|coding|software|developer)\b`),
		"api":      regexp.MustCompile(`(?i)\b(api|endpoint|rest|graphql|grpc)\b`),
		"database": regexp.MustCompile(`(?i)\b(database|sql|postgres|mysql|sqlite|mongo)\b`),
		"cloud":    regexp.MustCompile(`(?i)\b(aws|gcp|azure|cloud|kubernetes|docker)\b`),
		"language": regexp.MustCompile(`(?i)\b(go|golang|python|javascript|typescript|rust|java|c\+\+)\b`),
		"work":     regexp.MustCompile(`(?i)\b(job|work|company|team|manager|colleague)\b`),
		"personal": regexp.MustCompile(`(?i)\b(family|hobby|pet|home|vacation|travel)\b`),
	}

	var tags []string
	for tag, pattern := range tagPatterns {
		if pattern.MatchString(text) {
			tags = append(tags, tag)
		}
	}
	return tags
}

func dedup(memories []ExtractedMemory) []ExtractedMemory {
	seen := make(map[string]struct{})
	var unique []ExtractedMemory
	for _, m := range memories {
		key := string(m.Type) + ":" + m.Content
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, m)
	}
	return unique
}
