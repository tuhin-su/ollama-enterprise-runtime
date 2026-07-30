package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

var displayCmd = &cobra.Command{
	Use:     "display [MODEL] [PROMPT]",
	Short:   "Run a model in display mode with no chat interface",
	Args:    cobra.MinimumNArgs(0),
	PreRunE: checkServerHeartbeat,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.ClientFromEnvironment()
		if err != nil {
			return err
		}

		model := "llama3.2" // default model
		if len(args) > 0 {
			model = args[0]
		}

		messages := []api.Message{
			{
				Role:    "system",
				Content: "You are an autonomous background agent. You do not have a direct text channel to the user. You should use your websocket-provided tools to display text or request input.",
			},
		}

		prompt := "Hello"
		if len(args) > 1 {
			prompt = strings.Join(args[1:], " ")
		}

		messages = append(messages, api.Message{
			Role:    "user",
			Content: prompt,
		})

		for {
			req := &api.ChatRequest{
				Model:    model,
				Messages: messages,
			}

			var lastResp *api.ChatResponse
			err = client.Chat(context.Background(), req, func(resp api.ChatResponse) error {
				if resp.Done {
					lastResp = &resp
					messages = append(messages, resp.Message)
				}
				return nil
			})
			if err != nil {
				return err
			}

			if lastResp == nil || len(lastResp.Message.ToolCalls) == 0 {
				break // Done with tool loop, go back to waiting for outer input
			}

			for _, tc := range lastResp.Message.ToolCalls {
				var resultStr string
				var args map[string]any
				b, _ := json.Marshal(tc.Function.Arguments)
				_ = json.Unmarshal(b, &args)

				if tc.Function.Name == "display" {
					if data, ok := args["data"].(string); ok {
						out, err := glamour.Render(data, "dark")
						if err == nil {
							fmt.Print(out)
						} else {
							fmt.Println("\n--- DISPLAY ---")
							fmt.Println(data)
							fmt.Println("---------------")
						}
						resultStr = "Displayed successfully."
					}
				}

				messages = append(messages, api.Message{
					Role:       "tool",
					Content:    resultStr,
					ToolCallID: tc.ID,
				})
			}
		}

		return nil
	},
}
