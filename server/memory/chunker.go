package memory

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// DocumentChunk represents a chunked segment of a document.
type DocumentChunk struct {
	Index      int               `json:"index"`
	Content    string            `json:"content"`
	Source     string            `json:"source"`
	PageNumber int               `json:"page_number,omitempty"`
	StartLine  int               `json:"start_line,omitempty"`
	EndLine    int               `json:"end_line,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ChunkingStrategy defines the algorithm used for document chunking.
type ChunkingStrategy string

const (
	StrategyRecursive ChunkingStrategy = "recursive"
	StrategySliding   ChunkingStrategy = "sliding"
	StrategyMarkdown  ChunkingStrategy = "markdown"
)

// ChunkOptions configures document chunking.
type ChunkOptions struct {
	ChunkSize    int              `json:"chunk_size"`
	ChunkOverlap int              `json:"chunk_overlap"`
	Strategy     ChunkingStrategy `json:"strategy"`
	SourcePath   string           `json:"source_path"`
}

// DefaultChunkOptions returns production defaults.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		ChunkSize:    512,
		ChunkOverlap: 64,
		Strategy:     StrategyRecursive,
	}
}

// DocumentChunker chunks raw document text into optimized RAG segments.
type DocumentChunker struct{}

func NewDocumentChunker() *DocumentChunker {
	return &DocumentChunker{}
}

// ChunkText splits raw text according to the specified options.
func (c *DocumentChunker) ChunkText(text string, opts ChunkOptions) []DocumentChunk {
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = 512
	}
	if opts.ChunkOverlap < 0 || opts.ChunkOverlap >= opts.ChunkSize {
		opts.ChunkOverlap = 64
	}

	ext := strings.ToLower(filepath.Ext(opts.SourcePath))
	if opts.Strategy == "" {
		if ext == ".md" || ext == ".markdown" {
			opts.Strategy = StrategyMarkdown
		} else {
			opts.Strategy = StrategyRecursive
		}
	}

	switch opts.Strategy {
	case StrategyMarkdown:
		return c.chunkMarkdown(text, opts)
	case StrategySliding:
		return c.chunkSliding(text, opts)
	default:
		return c.chunkRecursive(text, opts)
	}
}

func (c *DocumentChunker) chunkRecursive(text string, opts ChunkOptions) []DocumentChunk {
	separators := []string{"\n\n", "\n", ". ", " ", ""}
	rawChunks := splitTextRecursive(text, separators, opts.ChunkSize, opts.ChunkOverlap)

	chunks := make([]DocumentChunk, 0, len(rawChunks))
	currentLine := 1

	for i, rc := range rawChunks {
		lines := strings.Count(rc, "\n")
		endLine := currentLine + lines

		chunks = append(chunks, DocumentChunk{
			Index:     i,
			Content:   strings.TrimSpace(rc),
			Source:    opts.SourcePath,
			StartLine: currentLine,
			EndLine:   endLine,
			Metadata: map[string]string{
				"strategy": string(StrategyRecursive),
			},
		})
		currentLine = endLine
	}
	return chunks
}

func (c *DocumentChunker) chunkSliding(text string, opts ChunkOptions) []DocumentChunk {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	step := opts.ChunkSize - opts.ChunkOverlap
	if step <= 0 {
		step = opts.ChunkSize / 2
	}

	var chunks []DocumentChunk
	idx := 0

	for i := 0; i < len(words); i += step {
		end := i + opts.ChunkSize
		if end > len(words) {
			end = len(words)
		}

		chunkText := strings.Join(words[i:end], " ")
		chunks = append(chunks, DocumentChunk{
			Index:   idx,
			Content: chunkText,
			Source:  opts.SourcePath,
			Metadata: map[string]string{
				"strategy": string(StrategySliding),
			},
		})
		idx++
		if end == len(words) {
			break
		}
	}
	return chunks
}

func (c *DocumentChunker) chunkMarkdown(text string, opts ChunkOptions) []DocumentChunk {
	headerRegex := regexp.MustCompile(`(?m)^(#{1,6})\s+(.+)$`)
	locs := headerRegex.FindAllStringIndex(text, -1)

	if len(locs) == 0 {
		return c.chunkRecursive(text, opts)
	}

	var chunks []DocumentChunk
	chunkIdx := 0

	for i := 0; i < len(locs); i++ {
		start := locs[i][0]
		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}

		section := text[start:end]
		lines := strings.Split(section, "\n")
		header := strings.TrimSpace(lines[0])

		if len(section) > opts.ChunkSize {
			subChunks := c.chunkRecursive(section, opts)
			for _, sc := range subChunks {
				sc.Index = chunkIdx
				if sc.Metadata == nil {
					sc.Metadata = make(map[string]string)
				}
				sc.Metadata["header"] = header
				chunks = append(chunks, sc)
				chunkIdx++
			}
		} else {
			chunks = append(chunks, DocumentChunk{
				Index:   chunkIdx,
				Content: strings.TrimSpace(section),
				Source:  opts.SourcePath,
				Metadata: map[string]string{
					"header":   header,
					"strategy": string(StrategyMarkdown),
				},
			})
			chunkIdx++
		}
	}

	return chunks
}

func splitTextRecursive(text string, separators []string, chunkSize, overlap int) []string {
	if len(text) <= chunkSize || len(separators) == 0 {
		return []string{text}
	}

	sep := separators[0]
	nextSeps := separators[1:]

	parts := strings.Split(text, sep)
	var result []string
	var current strings.Builder

	for _, part := range parts {
		candidate := part
		if current.Len() > 0 {
			candidate = sep + part
		}

		if current.Len()+len(candidate) <= chunkSize {
			current.WriteString(candidate)
		} else {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}

			if len(part) > chunkSize {
				subParts := splitTextRecursive(part, nextSeps, chunkSize, overlap)
				result = append(result, subParts...)
			} else {
				current.WriteString(part)
			}
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// FormatCitation formats a document chunk into an enterprise citation string.
func (c *DocumentChunk) FormatCitation() string {
	var parts []string
	if c.Source != "" {
		parts = append(parts, fmt.Sprintf("Source: %s", filepath.Base(c.Source)))
	}
	if c.PageNumber > 0 {
		parts = append(parts, fmt.Sprintf("Page: %d", c.PageNumber))
	}
	if c.StartLine > 0 && c.EndLine > 0 {
		parts = append(parts, fmt.Sprintf("Lines: %d-%d", c.StartLine, c.EndLine))
	}
	if header, ok := c.Metadata["header"]; ok && header != "" {
		parts = append(parts, fmt.Sprintf("Section: %s", header))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("[Chunk #%d]", c.Index)
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, " | "))
}
