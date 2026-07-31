package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/gorilla/websocket"
	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

var inputCmd = &cobra.Command{
	Use:   "input <model>",
	Short: "Take user input and send it to the model",
	Args:  cobra.ExactArgs(1),
	PreRunE: checkServerHeartbeat,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.ClientFromEnvironment()
		if err != nil {
			return err
		}
		model := args[0]

		messages := []api.Message{}

		fmt.Println("Ready for input. The model will display output in the registered display tool.")
		scanner := bufio.NewScanner(os.Stdin)

		for {
			fmt.Print(">> ")
			if !scanner.Scan() {
				break
			}
			text := scanner.Text()
			if text == "" {
				continue
			}

			messages = append(messages, api.Message{
				Role:    "user",
				Content: text,
			})

			// Add instructions to use the display tool
			props := api.NewToolPropertiesMap()
			props.Set("data", api.ToolProperty{
				Type:        api.PropertyType{"string"},
				Description: "The markdown text or data to display to the user.",
			})

			req := &api.ChatRequest{
				Model:    model,
				Messages: messages,
				Options: map[string]interface{}{
					"num_predict": 8192,
					"num_ctx":     32768,
				},
				Tools: []api.Tool{
					{
						Type: "function",
						Function: api.ToolFunction{
							Name:        "display",
							Description: "Display text to the user beautifully as markdown. Use this tool for visibility, troubleshooting, UI interface, or to show any response to the user. Returns a success acknowledgement when done.",
							Parameters: api.ToolFunctionParameters{
								Type:       "object",
								Properties: props,
								Required:   []string{"data"},
							},
						},
					},
				},
			}

			stream := false
			req.Stream = &stream

			var fullResponse string
			var toolCalls []api.ToolCall

			err = client.Chat(context.Background(), req, func(resp api.ChatResponse) error {
				if resp.Message.Content != "" {
					fullResponse += resp.Message.Content
				}
				if len(resp.Message.ToolCalls) > 0 {
					toolCalls = append(toolCalls, resp.Message.ToolCalls...)
				}
				if resp.Done {
					messages = append(messages, resp.Message)
				}
				return nil
			})
			if err != nil {
				fmt.Println("Error:", err)
				continue
			}

			// For now, if the model used the display tool, we just manually trigger a websocket to it, or print it.
			// Since we want the display tool to show it, we can connect to the tool interface.
			if len(toolCalls) > 0 {
				for _, tc := range toolCalls {
					if tc.Function.Name == "display" {
						// Extract data argument
						args := tc.Function.Arguments.ToMap()
						// Send to display tool via WS
						sendToToolInterface(tc.Function.Name, args)
					}
				}
			} else if fullResponse != "" {
				// Fallback if model didn't use the tool
				fmt.Println(fullResponse)
			}
			fmt.Println()
		}

		return scanner.Err()
	},
}

func sendToToolInterface(targetTool string, payload map[string]interface{}) {
	u := "ws://127.0.0.1:11434/api/tools/interface"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		fmt.Println("Error connecting to display tool interface:", err)
		return
	}
	defer conn.Close()

	msg := map[string]interface{}{
		"type": "model_call_tool",
		"tool_name": targetTool,
		"tool_arguments": payload,
	}

	err = conn.WriteJSON(msg)
	if err != nil {
		fmt.Println("Error sending to display tool:", err)
	}
}
