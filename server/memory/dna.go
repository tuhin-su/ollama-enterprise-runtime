package memory

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// DynamicProfile (formerly DNA) defines the system's operational profile and resource parameters.
// Stored persistently at ~/.loom/dna/profile.json
type DynamicProfile struct {
	mu sync.RWMutex

	// System Identification
	Version         string `json:"version"`
	BehaviorProfile string `json:"behavior_profile"` // e.g. "analytical", "concise", "creative"
	
	// Behavioral Rules & User Storage Protocols
	BehavioralRules  []string `json:"behavioral_rules"`
	StorageProtocols []string `json:"storage_protocols"`

	// Dynamic Learning & Execution Traits
	ConfidenceThreshold  float64 `json:"confidence_threshold"` // Below this, ask user for clarification
	EnableSystemCheck    bool    `json:"enable_system_check"`   // Inspect memory/CPU before execution
	AutoAskUserOnMissing bool    `json:"auto_ask_user_on_missing"` // Prompt user when facts are unknown

	// Environment & Hardware Snapshot
	AllocatedRAMMB uint64 `json:"allocated_ram_mb"`
	CPUCoreCount   int    `json:"cpu_core_count"`
	TargetVRAMMB   uint64 `json:"target_vram_mb"`

	filePath string
}

// SystemResourceStatus holds real-time memory and CPU resource checks.
type SystemResourceStatus struct {
	AllocatedSysRAMMB uint64 `json:"allocated_sys_ram_mb"`
	HeapAllocMB       uint64 `json:"heap_alloc_mb"`
	NumGoroutine      int    `json:"num_goroutine"`
	CPUCount          int    `json:"cpu_count"`
	SufficientRAM     bool   `json:"sufficient_ram"`
}

// LoadOrCreateProfile initializes or loads the persistent profile from ~/.loom/dna/profile.json
func LoadOrCreateProfile() (*DynamicProfile, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	dnaDir := filepath.Join(home, ".loom", "dna")
	if err := os.MkdirAll(dnaDir, 0755); err != nil {
		return nil, fmt.Errorf("dna profile: failed to create directory: %w", err)
	}

	profilePath := filepath.Join(dnaDir, "profile.json")
	p := &DynamicProfile{
		Version:         "1.0.0",
		BehaviorProfile: "analytical",
		BehavioralRules: []string{
			"Always maintain high accuracy and prioritize verifiable facts over guesses.",
			"When user shares critical data (credentials, keys, preferences), automatically save to long-term memory.",
			"If confidence is low or required parameters are missing, ask the user for explicit clarification.",
		},
		StorageProtocols: []string{
			"Pin important memories when explicitly marked as high priority by the user.",
			"Tag user preferences as 'user_preference' and project constraints as 'project_rule'.",
		},
		ConfidenceThreshold:  0.65,
		EnableSystemCheck:    true,
		AutoAskUserOnMissing: true,
		CPUCoreCount:         runtime.NumCPU(),
		filePath:             profilePath,
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			p.updateHardwareSnapshot()
			_ = p.Save()
			slog.Info("dna profile: auto-generated initial profile", "path", profilePath)
			return p, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, p); err != nil {
		slog.Warn("dna profile: failed to parse profile.json, using default snapshot", "error", err)
	}
	p.filePath = profilePath
	p.updateHardwareSnapshot()
	return p, nil
}

func (p *DynamicProfile) updateHardwareSnapshot() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	p.AllocatedRAMMB = m.Sys / (1024 * 1024)
	p.CPUCoreCount = runtime.NumCPU()
}

// Save persists the dynamic profile back to disk.
func (p *DynamicProfile) Save() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	bytes, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.filePath, bytes, 0644)
}

// CheckSystemResources queries current Go runtime memory and host CPU stats.
func (p *DynamicProfile) CheckSystemResources() SystemResourceStatus {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	allocMB := m.Alloc / (1024 * 1024)
	sysMB := m.Sys / (1024 * 1024)

	return SystemResourceStatus{
		AllocatedSysRAMMB: sysMB,
		HeapAllocMB:       allocMB,
		NumGoroutine:      runtime.NumGoroutine(),
		CPUCount:          runtime.NumCPU(),
		SufficientRAM:     sysMB < 16384, // Returns true if process RAM usage is within normal bounds
	}
}

// EvaluateKnowledgeConfidence evaluates whether a prompt query has sufficient context or should ask the user for clarification.
func (p *DynamicProfile) EvaluateKnowledgeConfidence(matchedScore float64, factCount int) (bool, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if factCount == 0 || matchedScore < p.ConfidenceThreshold {
		if p.AutoAskUserOnMissing {
			return false, "Confidence threshold not met. Ask user for additional context or clarification."
		}
	}
	return true, "Sufficient context available."
}
