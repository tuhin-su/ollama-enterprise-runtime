package server

import (
	"os"
	"path/filepath"
)

var debugLogChan = make(chan string, 5000)

func init() {
	go func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		logPath := filepath.Join(home, ".ollama", "debug.log")
		os.MkdirAll(filepath.Dir(logPath), 0755)
		
		// Truncate on start
		f, err := os.OpenFile(logPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		
		for msg := range debugLogChan {
			f.WriteString(msg)
			f.Sync()
		}
	}()
}

// WriteDebug appends text to the global debug log safely.
func WriteDebug(text string) {
	select {
	case debugLogChan <- text:
	default:
		// Drop if full to avoid blocking server
	}
}
