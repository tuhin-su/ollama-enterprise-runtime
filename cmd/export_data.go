package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loom/loom/server/memory"
	"github.com/spf13/cobra"
)

// SSLTrainingExample defines the schema for self-supervised pre-training or instruction fine-tuning.
type SSLTrainingExample struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`       // "unsupervised_text", "instruction_response", "memory_fact"
	Prompt    string   `json:"prompt,omitempty"`
	Completion string  `json:"completion,omitempty"`
	Text      string   `json:"text,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

var exportDataCmd = &cobra.Command{
	Use:   "export-data [output-file.jsonl]",
	Short: "Export interaction histories and long-term memory for Self-Supervised Learning (SSL) fine-tuning",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputPath := "loom_ssl_training_dataset.jsonl"
		if len(args) > 0 && args[0] != "" {
			outputPath = args[0]
		}

		fmt.Printf("Exporting dataset for Self-Supervised Learning to '%s'...\n", outputPath)

		cfg := memory.LoadConfig()
		store, err := memory.NewLanceDBStore(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("failed to open memory store: %w", err)
		}
		defer store.Close()

		ctx := context.Background()
		memories, err := store.List(ctx, "", memory.ListOptions{Limit: 100000})
		if err != nil {
			return fmt.Errorf("failed to list memories: %w", err)
		}

		outFile, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()

		exportedCount := 0

		for _, mem := range memories {
			ex := SSLTrainingExample{
				ID:        mem.ID,
				Type:      string(mem.Type),
				Text:      mem.Content,
				Tags:      mem.Tags,
			}

			if mem.Summary != "" {
				ex.Prompt = fmt.Sprintf("Summarize the following %s content:\n%s", mem.Type, mem.Content)
				ex.Completion = mem.Summary
				ex.Type = "instruction_response"
			} else {
				ex.Type = "unsupervised_text"
			}

			data, err := json.Marshal(ex)
			if err != nil {
				continue
			}

			outFile.Write(data)
			outFile.WriteString("\n")
			exportedCount++
		}

		absPath, _ := filepath.Abs(outputPath)
		fmt.Printf("Successfully exported %d self-supervised training records to: %s\n", exportedCount, absPath)
		return nil
	},
}
