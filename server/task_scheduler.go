package server

// task_scheduler.go — Scheduled task engine for Loom.
//
// Allows models (and users) to schedule prompts to run against any local model
// at a specific time, after a delay, or on a recurring cron schedule.
//
// Supported run_at formats:
//   - RFC3339 timestamp   "2026-01-15T14:30:00Z"
//   - Relative delay      "in 5m", "in 2h", "in 1h30m"
//   - Duration shorthand  "5m", "2h30m"
// Recurring jobs use a standard 5-field cron expression in the cron field.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loom/loom/api"
	"github.com/loom/loom/llm"
	"github.com/loom/loom/types/model"
)

// ─────────────────────────────────────────────────────────────────────────────
// Domain types
// ─────────────────────────────────────────────────────────────────────────────

// JobStatus is the lifecycle state of a scheduled job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusDone      JobStatus = "done"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

// doneJobRetention is how long a completed one-shot job's result is kept before
// the job entry is automatically removed from the store.
// Set to 0 to keep forever (manual delete required).
const doneJobRetention = 24 * time.Hour

// ScheduledJob is a single unit of deferred work.
type ScheduledJob struct {
	// ID is a unique system-assigned identifier.
	ID string `json:"id"`
	// PromptID is a human-readable label provided by the caller.
	PromptID string `json:"prompt_id"`
	// Prompt is the user message sent to the model.
	Prompt string `json:"prompt"`
	// Model is the Loom model to use. Empty = server default model.
	Model string `json:"model,omitempty"`
	// SystemPrompt is an optional system message.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// RunAt is when the job fires (one-shot).
	RunAt time.Time `json:"run_at"`
	// CronExpr is a 5-field cron expression for recurring jobs.
	CronExpr string `json:"cron_expr,omitempty"`

	Status      JobStatus  `json:"status"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// NextRunAt is set for recurring jobs after each execution.
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
	// RunCount tracks how many times a recurring job has fired.
	RunCount int `json:"run_count,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// TaskScheduler
// ─────────────────────────────────────────────────────────────────────────────

// TaskScheduler manages and executes scheduled jobs.
type TaskScheduler struct {
	mu       sync.Mutex
	jobs     map[string]*ScheduledJob
	filePath string
	server   *Server
	cancel   context.CancelFunc
}

// globalScheduler is the server-wide singleton.
var globalScheduler *TaskScheduler

// initTaskScheduler creates and starts the task scheduler singleton.
func initTaskScheduler(ctx context.Context, s *Server) {
	if globalScheduler != nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("scheduler: cannot find home dir", "error", err)
		return
	}
	ts := &TaskScheduler{
		jobs:     make(map[string]*ScheduledJob),
		filePath: filepath.Join(home, ".loom", "scheduler.json"),
		server:   s,
	}
	ts.load()

	schedCtx, cancel := context.WithCancel(ctx)
	ts.cancel = cancel
	globalScheduler = ts

	go ts.run(schedCtx)
	slog.Info("task scheduler started", "store", ts.filePath, "jobs", len(ts.jobs))
}

// shutdownTaskScheduler stops the background goroutine.
func shutdownTaskScheduler() {
	if globalScheduler != nil && globalScheduler.cancel != nil {
		globalScheduler.cancel()
	}
}

// run is the background ticker loop.
func (ts *TaskScheduler) run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ts.tick(ctx) // check immediately on start

	for {
		select {
		case <-ctx.Done():
			slog.Info("task scheduler: stopped")
			return
		case <-ticker.C:
			ts.tick(ctx)
		}
	}
}

// tick fires any jobs whose RunAt has arrived and prunes expired results.
func (ts *TaskScheduler) tick(ctx context.Context) {
	now := time.Now()
	ts.mu.Lock()
	var due []*ScheduledJob
	var toDelete []string
	for _, j := range ts.jobs {
		// Fire pending jobs whose time has come
		if j.Status == JobStatusPending && !j.RunAt.IsZero() && !now.Before(j.RunAt) {
			due = append(due, j)
			continue
		}
		// Auto-remove one-shot done/failed jobs after retention window
		if doneJobRetention > 0 && j.CronExpr == "" &&
			(j.Status == JobStatusDone || j.Status == JobStatusFailed) &&
			j.CompletedAt != nil && now.Sub(*j.CompletedAt) > doneJobRetention {
			toDelete = append(toDelete, j.ID)
		}
	}
	for _, id := range toDelete {
		slog.Info("scheduler: auto-removing expired job", "id", id)
		delete(ts.jobs, id)
	}
	if len(toDelete) > 0 {
		ts.save()
	}
	ts.mu.Unlock()

	for _, j := range due {
		jobCopy := j
		go ts.executeJob(ctx, jobCopy)
	}
}

// executeJob runs one job, stores the result, and (for cron jobs) resets RunAt.
func (ts *TaskScheduler) executeJob(ctx context.Context, job *ScheduledJob) {
	ts.mu.Lock()
	// Re-check: may have been cancelled between tick and goroutine start
	live := ts.jobs[job.ID]
	if live == nil || live.Status != JobStatusPending {
		ts.mu.Unlock()
		return
	}
	live.Status = JobStatusRunning
	now := time.Now()
	live.StartedAt = &now
	ts.mu.Unlock()

	slog.Info("scheduler: running job",
		"id", job.ID,
		"prompt_id", job.PromptID,
		"model", job.Model,
	)

	// Acquire exclusive VRAM lock before executing background model execution
	VRAMArbiter.Lock()
	result, execErr := ts.runPrompt(ctx, job.Model, job.SystemPrompt, job.Prompt)
	VRAMArbiter.Unlock()

	ts.mu.Lock()
	live = ts.jobs[job.ID]
	if live == nil {
		ts.mu.Unlock()
		return
	}
	done := time.Now()
	live.CompletedAt = &done
	live.RunCount++

	if execErr != nil {
		live.Status = JobStatusFailed
		live.Error = execErr.Error()
		slog.Error("scheduler: job failed", "id", job.ID, "error", execErr)
	} else {
		live.Status = JobStatusDone
		live.Result = result
		slog.Info("scheduler: job done", "id", job.ID, "result_len", len(result))
	}

	// Recurring jobs: reset to pending with next RunAt
	if live.CronExpr != "" {
		next, cronErr := nextCronTime(live.CronExpr, done)
		if cronErr == nil {
			live.Status = JobStatusPending
			live.RunAt = next
			live.NextRunAt = &next
		}
	}

	ts.save()
	ts.mu.Unlock()
}

// runPrompt executes a prompt against a model using the scheduler's runner.
func (ts *TaskScheduler) runPrompt(ctx context.Context, modelName, systemPrompt, userPrompt string) (string, error) {
	if ts.server == nil {
		return "", fmt.Errorf("server not initialised")
	}

	// Resolve model
	if modelName == "" {
		modelName = schedulerDefaultModel()
	}
	if modelName == "" {
		return "", fmt.Errorf("no model specified and no default_model configured")
	}

	mName := model.ParseName(modelName)
	mName, err := getExistingName(mName)
	if err != nil {
		return "", fmt.Errorf("model '%s' not found: %w", modelName, err)
	}

	m, err := GetModel(mName.String())
	if err != nil {
		return "", fmt.Errorf("load model '%s': %w", modelName, err)
	}

	caps := []model.Capability{model.CapabilityCompletion}
	r, m, opts, err := ts.server.scheduleRunner(ctx, mName.String(), caps, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("schedule runner '%s': %w", modelName, err)
	}

	// Build messages
	msgs := []api.Message{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		msgs = append([]api.Message{{Role: "system", Content: systemPrompt}}, msgs...)
	} else if m.System != "" {
		msgs = append([]api.Message{{Role: "system", Content: m.System}}, msgs...)
	}

	promptOpts := optionsForPrompt(opts, r)
	templatePrompt, _, err := chatPrompt(ctx, m, r.Tokenize, promptOpts, msgs, nil, nil, true)
	if err != nil {
		return "", fmt.Errorf("build prompt: %w", err)
	}

	var output strings.Builder
	if err := r.Completion(ctx, llm.CompletionRequest{
		Prompt:   templatePrompt,
		Options:  opts,
		Shift:    true,
		Truncate: true,
	}, func(resp llm.CompletionResponse) {
		output.WriteString(resp.Content)
	}); err != nil {
		return "", fmt.Errorf("completion: %w", err)
	}
	return output.String(), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CRUD
// ─────────────────────────────────────────────────────────────────────────────

// AddJob validates and persists a new job, returning its ID.
func (ts *TaskScheduler) AddJob(job *ScheduledJob) (string, error) {
	if strings.TrimSpace(job.Prompt) == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if job.RunAt.IsZero() && job.CronExpr == "" {
		return "", fmt.Errorf("either run_at or cron is required")
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	job.ID = fmt.Sprintf("job-%d", time.Now().UnixNano())
	if job.PromptID == "" {
		job.PromptID = job.ID
	}
	job.Status = JobStatusPending
	job.CreatedAt = time.Now()

	// Compute first RunAt for cron jobs
	if job.CronExpr != "" && job.RunAt.IsZero() {
		next, err := nextCronTime(job.CronExpr, time.Now())
		if err != nil {
			return "", fmt.Errorf("invalid cron: %w", err)
		}
		job.RunAt = next
		job.NextRunAt = &next
	}

	ts.jobs[job.ID] = job
	ts.save()
	return job.ID, nil
}

// CancelJob marks a pending job as cancelled.
func (ts *TaskScheduler) CancelJob(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	j, ok := ts.jobs[id]
	if !ok {
		return fmt.Errorf("job '%s' not found", id)
	}
	if j.Status != JobStatusPending {
		return fmt.Errorf("job '%s' is %s, cannot cancel", id, j.Status)
	}
	j.Status = JobStatusCancelled
	ts.save()
	return nil
}

// DeleteJob removes a job entirely.
func (ts *TaskScheduler) DeleteJob(id string) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if _, ok := ts.jobs[id]; !ok {
		return fmt.Errorf("job '%s' not found", id)
	}
	delete(ts.jobs, id)
	ts.save()
	return nil
}

// GetJob returns a copy of the job by ID (nil if not found).
func (ts *TaskScheduler) GetJob(id string) *ScheduledJob {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	j, ok := ts.jobs[id]
	if !ok {
		return nil
	}
	cp := *j
	return &cp
}

// ListJobs returns all jobs optionally filtered by status string.
func (ts *TaskScheduler) ListJobs(statusFilter string) []*ScheduledJob {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	var out []*ScheduledJob
	for _, j := range ts.jobs {
		if statusFilter == "" || string(j.Status) == statusFilter {
			cp := *j
			out = append(out, &cp)
		}
	}
	// Sort by RunAt ascending
	sort.Slice(out, func(i, k int) bool {
		return out[i].RunAt.Before(out[k].RunAt)
	})
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
// Persistence helpers
// ─────────────────────────────────────────────────────────────────────────────

type schedulerStore struct {
	Jobs []*ScheduledJob `json:"jobs"`
}

func (ts *TaskScheduler) save() {
	store := schedulerStore{}
	for _, j := range ts.jobs {
		store.Jobs = append(store.Jobs, j)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		slog.Warn("scheduler: marshal failed", "error", err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(ts.filePath), 0o755)
	if err := os.WriteFile(ts.filePath, data, 0o644); err != nil {
		slog.Warn("scheduler: write failed", "error", err)
	}
}

func (ts *TaskScheduler) load() {
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("scheduler: read failed", "error", err)
		}
		return
	}
	var store schedulerStore
	if err := json.Unmarshal(data, &store); err != nil {
		slog.Warn("scheduler: parse failed", "error", err)
		return
	}
	for _, j := range store.Jobs {
		// Jobs that were Running when the server stopped are marked failed
		if j.Status == JobStatusRunning {
			j.Status = JobStatusFailed
			j.Error = "server restarted during execution"
		}
		ts.jobs[j.ID] = j
	}
	slog.Info("scheduler: loaded persisted jobs", "count", len(ts.jobs))
}

// ─────────────────────────────────────────────────────────────────────────────
// Cron parser (5-field: minute hour dom month dow)
// ─────────────────────────────────────────────────────────────────────────────

// nextCronTime returns the next time after `after` that satisfies `expr`.
func nextCronTime(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron must have 5 fields (minute hour dom month dow), got %d", len(fields))
	}
	minuteSet, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("minute field: %w", err)
	}
	hourSet, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("hour field: %w", err)
	}
	domSet, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("dom field: %w", err)
	}
	monthSet, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("month field: %w", err)
	}
	dowSet, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("dow field: %w", err)
	}

	t := after.Add(time.Minute).Truncate(time.Minute)
	limit := after.Add(5 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		if !monthSet[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !domSet[t.Day()] || !dowSet[int(t.Weekday())] {
			t = t.Add(24 * time.Hour).Truncate(24 * time.Hour)
			continue
		}
		if !hourSet[t.Hour()] {
			t = t.Add(time.Hour).Truncate(time.Hour)
			continue
		}
		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cron expression '%s' never fires within 5 years", expr)
}

// parseCronField turns a single cron field into a boolean set.
func parseCronField(field string, min, max int) (map[int]bool, error) {
	set := make(map[int]bool)
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			for i := min; i <= max; i++ {
				set[i] = true
			}
			continue
		}
		if strings.Contains(part, "/") {
			// Step: */n or start/n or start-end/n
			halves := strings.SplitN(part, "/", 2)
			step, err := strconv.Atoi(halves[1])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step in '%s'", part)
			}
			start, end := min, max
			if halves[0] != "*" {
				if strings.Contains(halves[0], "-") {
					r := strings.SplitN(halves[0], "-", 2)
					if start, err = strconv.Atoi(r[0]); err != nil {
						return nil, fmt.Errorf("invalid range '%s'", halves[0])
					}
					if end, err = strconv.Atoi(r[1]); err != nil {
						return nil, fmt.Errorf("invalid range '%s'", halves[0])
					}
				} else {
					if start, err = strconv.Atoi(halves[0]); err != nil {
						return nil, fmt.Errorf("invalid start '%s'", halves[0])
					}
					end = max
				}
			}
			for i := start; i <= end; i += step {
				if i >= min && i <= max {
					set[i] = true
				}
			}
			continue
		}
		if strings.Contains(part, "-") {
			// Range: n-m
			r := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(r[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start '%s'", r[0])
			}
			end, err := strconv.Atoi(r[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end '%s'", r[1])
			}
			for i := start; i <= end; i++ {
				set[i] = true
			}
			continue
		}
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value '%s'", part)
		}
		if v < min || v > max {
			return nil, fmt.Errorf("value %d out of range [%d,%d]", v, min, max)
		}
		set[v] = true
	}
	return set, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Time parsing helper
// ─────────────────────────────────────────────────────────────────────────────

// ParseScheduleTime converts user-provided time specs into a wall-clock time.
//
// Accepted formats:
//   - RFC3339              "2026-01-15T14:30:00Z"
//   - Duration (relative)  "5m", "2h", "1h30m", "in 5m", "in 2 hours"
func ParseScheduleTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("time string is empty")
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Strip "in "
	normalized := strings.TrimPrefix(strings.ToLower(s), "in ")
	normalized = strings.TrimSpace(normalized)

	// Normalise common human words to Go duration tokens
	replacer := strings.NewReplacer(
		" hours", "h", " hour", "h",
		" minutes", "m", " minute", "m",
		" seconds", "s", " second", "s",
		" ", "",
	)
	normalized = replacer.Replace(normalized)

	if d, err := time.ParseDuration(normalized); err == nil {
		if d <= 0 {
			return time.Time{}, fmt.Errorf("delay must be positive, got %s", s)
		}
		return time.Now().Add(d), nil
	}
	return time.Time{}, fmt.Errorf("unrecognised time format %q — use RFC3339 or a duration like '5m', '2h30m', 'in 1 hour'", s)
}

// schedulerDefaultModel reads the default_model from server.json.
func schedulerDefaultModel() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".loom", "server.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		DefaultModel string `json:"default_model"`
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg.DefaultModel
}
