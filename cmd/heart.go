package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/loom/loom/api"
)

var heartCmd = &cobra.Command{
	Use:   "heart",
	Short: "Send a heartbeat pulse to all loaded models when they are free",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.ClientFromEnvironment()
		if err != nil {
			return err
		}

		cfg := memory.LoadConfig()
		intervalSec := cfg.HeartbeatIntervalSeconds
		if intervalSec <= 0 {
			intervalSec = 30
		}
		fmt.Printf("Starting 24/7 autonomous heartbeat monitor for all models (interval: %ds)...\n", intervalSec)

		for {
			time.Sleep(time.Duration(intervalSec) * time.Second)

			// 1. Get all loaded models
			psResp, err := client.ListRunning(context.Background())
			if err != nil {
				continue
			}

			if len(psResp.Models) == 0 {
				// No models currently loaded
				continue
			}

			for _, m := range psResp.Models {
				modelName := m.Name
				fmt.Printf("Sending pulse to loaded model: %s\n", modelName)
				stream := false
				req := &api.ChatRequest{
					Model: modelName,
					Messages: []api.Message{
						{
							Role:    "system",
							Content: `[SYSTEM STATUS: IDLE CYCLE]
You are a highly advanced, proactive AI system (similar to an autonomous assistant). I am your system heartbeat, pinging you during your free time. 

You have full autonomy right now. Please engage your thinking process:
1. Check if there are any pending tasks, incomplete thoughts, or background simulations you'd like to work on.
2. If there is something you'd like to do, you may use this time to output your thoughts, process data, or experiment.
3. If your queues are empty and you believe system resources should be conserved, you may choose to shut down your current instance.

IMPORTANT: Do NOT use any tools. Do not call toolmanager.search. Output your thoughts directly as text.
To shut down, you must output the exact string "UNLOAD_MODEL" somewhere in your text response.`,
						},
					},
					Stream: &stream,
				}

				unloaded := false
				err = client.Chat(context.Background(), req, func(resp api.ChatResponse) error {
					if resp.Message.Content != "" {
						fmt.Print(resp.Message.Content)
						if strings.Contains(resp.Message.Content, "UNLOAD_MODEL") {
							unloaded = true
						}
					}
					return nil
				})
				fmt.Println()
				
				if err != nil {
					fmt.Printf("Error sending pulse to %s: %v\n", modelName, err)
					continue
				}

				// Check if model decided to unload
				if unloaded {
					fmt.Printf("Model %s decided to unload. Unloading...\n", modelName)
					unloadReq := &api.GenerateRequest{
						Model:     modelName,
						KeepAlive: &api.Duration{Duration: 0},
					}
					client.Generate(context.Background(), unloadReq, func(api.GenerateResponse) error { return nil })
				}
			}
		}
	},
}
