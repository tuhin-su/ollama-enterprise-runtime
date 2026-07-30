package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"github.com/ollama/ollama/api"
)

var inputCmd = &cobra.Command{
	Use:   "input [MODEL]",
	Short: "Run a model in input mode",
	Args:  cobra.MinimumNArgs(0),
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

		fmt.Println("Listening for input on stdin (Input Mode)...")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := scanner.Text()
			if text == "" {
				continue
			}

			messages = append(messages, api.Message{
				Role:    "user",
				Content: text,
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
					} else if tc.Function.Name == "input" {
						if prompt, ok := args["prompt"].(string); ok {
							fmt.Print(prompt + " ")
							if scanner.Scan() {
								resultStr = scanner.Text()
							}
						}
					}

					messages = append(messages, api.Message{
						Role:       "tool",
						Content:    resultStr,
						ToolCallID: tc.ID,
					})
				}
			}
		}

		return scanner.Err()
	},
}
