package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

var inputCmd = &cobra.Command{
	Use:   "input",
	Short: "Take user input and send it to the model",
	Args:  cobra.NoArgs,
	PreRunE: checkServerHeartbeat,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.ClientFromEnvironment()
		if err != nil {
			return err
		}
		model := "default_model"

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

			req := &api.ChatRequest{
				Model:    model,
				Messages: messages,
				Options: map[string]interface{}{
					"num_predict": 8192,
					"num_ctx":     32768,
				},
			}

			// We don't print the response text because the model is instructed to use the display tool.
			err = client.Chat(context.Background(), req, func(resp api.ChatResponse) error {
				if resp.Done {
					messages = append(messages, resp.Message)
				}
				return nil
			})
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Println()
		}

		return scanner.Err()
	},
}
