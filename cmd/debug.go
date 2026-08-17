package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Directly tail the server's raw model inputs, outputs, and thinking processes.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		logPath := filepath.Join(home, ".loom", "debug.log")
		
		fmt.Printf("Intercepting raw model stream from %s...\n\n", logPath)
		
		c := exec.Command("tail", "-f", logPath)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}
