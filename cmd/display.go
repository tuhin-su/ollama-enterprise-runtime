package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/charmbracelet/glamour"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var displayCmd = &cobra.Command{
	Use:   "display",
	Short: "Register the display tool and wait for data",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		u := "ws://127.0.0.1:11434/api/tools/interface"
		c, _, err := websocket.DefaultDialer.Dial(u, nil)
		if err != nil {
			return fmt.Errorf("dial: %v", err)
		}
		defer c.Close()

		authMsg := map[string]interface{}{
			"auth_token": "abc",
			"role":       "tool",
			"tool_name":  "display",
			"tool_schema": map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        "display",
					"description": "Display text to the user beautifully as markdown. Use this tool for visibility, troubleshooting, UI interface, or to show any response to the user. Returns a success acknowledgement when done.",
					"parameters": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"data": map[string]interface{}{
								"type":        "string",
								"description": "The markdown text to display",
							},
						},
						"required": []string{"data"},
					},
				},
			},
		}

		if err := c.WriteJSON(authMsg); err != nil {
			return fmt.Errorf("write auth: %v", err)
		}

		var resp map[string]interface{}
		if err := c.ReadJSON(&resp); err != nil {
			return fmt.Errorf("read auth response: %v", err)
		}
		if status, _ := resp["status"].(string); status != "authenticated" {
			return fmt.Errorf("authentication failed: %v", resp)
		}

		fmt.Println("Registered display tool. Waiting for data...")

		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)

		go func() {
			for {
				var msg map[string]interface{}
				err := c.ReadJSON(&msg)
				if err != nil {
					log.Println("read error:", err)
					return
				}
				if t, _ := msg["type"].(string); t == "execute_tool" {
					reqID, _ := msg["request_id"].(string)
					payload, ok := msg["payload"].(map[string]interface{})
					
					var replyPayload map[string]string
					
					if ok {
						if data, ok := payload["data"].(string); ok && data != "" {
							out, err := glamour.Render(data, "dark")
							if err == nil {
								fmt.Print(out)
							} else {
								fmt.Println("\n--- DISPLAY ---")
								fmt.Println(data)
								fmt.Println("---------------")
							}
							replyPayload = map[string]string{"result": "Displayed successfully to the user. Acknowledgement received. No further response or plain text output is needed."}
						} else {
							fmt.Println("\n--- DISPLAY (TOOL CALL CUT OFF / ERROR) ---")
							fmt.Println("The model attempted to use the display tool, but the request was cut off or invalid.")
							fmt.Println("This usually happens if the model hits its token limit before finishing the JSON.")
							fmt.Printf("Raw Payload Received: %+v\n", payload)
							fmt.Println("-------------------------------------------")
							replyPayload = map[string]string{"error": "Missing 'data' argument in tool call. Your tool call was likely cut off because you hit the output limit, or it was invalid JSON. Please rectify your tool call and try again."}
						}
					} else {
						replyPayload = map[string]string{"error": "Invalid payload format."}
					}
					
					reply := map[string]interface{}{
						"type":        "tool_call_model",
						"request_id":  reqID,
						"source_tool": "display",
						"payload":     replyPayload,
					}
					c.WriteJSON(reply)
				}
			}
		}()

		<-interrupt
		fmt.Println("Shutting down display tool...")
		return nil
	},
}
