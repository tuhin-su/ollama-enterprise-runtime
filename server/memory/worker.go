package memory

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// WorkerPool is a fixed-size goroutine pool for background memory work
// (embedding generation, extraction, decay, cleanup).
type WorkerPool struct {
	jobs    chan func()
	wg      sync.WaitGroup
	running atomic.Bool
	size    int
}

// NewWorkerPool creates a pool with the given number of workers.
// Workers are not started until Start is called.
func NewWorkerPool(count int) *WorkerPool {
	if count < 1 {
		count = 1
	}
	return &WorkerPool{
		jobs: make(chan func(), count*64),
		size: count,
	}
}

// Start launches all workers.
func (p *WorkerPool) Start() {
	if p.running.Swap(true) {
		return // already started
	}

	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	slog.Info("memory worker pool started", "workers", p.size, "buffer", cap(p.jobs))
}

// Submit enqueues a job. Returns false if the pool is stopped or the
// job channel is full (non-blocking).
func (p *WorkerPool) Submit(job func()) bool {
	if !p.running.Load() {
		return false
	}
	select {
	case p.jobs <- job:
		return true
	default:
		slog.Warn("memory worker pool: job channel full, dropping job")
		return false
	}
}

// Stop closes the job channel and waits for all workers to finish.
func (p *WorkerPool) Stop() {
	if !p.running.Swap(false) {
		return // already stopped
	}
	close(p.jobs)
	p.wg.Wait()
	slog.Info("memory worker pool stopped")
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for job := range p.jobs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("memory worker panic recovered",
						"worker", id,
						"panic", r,
					)
				}
			}()
			job()
		}()
	}
}
