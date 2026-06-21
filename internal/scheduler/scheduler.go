// Package scheduler runs the cron-driven jobs (weekly digest, weekly
// poll, daily health). Wraps github.com/robfig/cron/v3 with a small
// interface so jobs can be unit-tested without time-based flakes.
package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/robfig/cron/v3"
)

// Job is one scheduled function. Run executes the body; Spec is a cron
// expression (5-field standard, e.g. "0 9 * * 0") or "" to disable.
type Job struct {
	Name string
	Spec string
	Run  func(context.Context) error
}

// Scheduler owns the cron loop. Add Jobs before Start; jobs added after
// Start are ignored (cron supports it but the design keeps it simple).
type Scheduler struct {
	c    *cron.Cron
	log  *slog.Logger
	jobs []Job
	mu   sync.Mutex
}

// New returns a scheduler using the standard 5-field cron parser.
func New(log *slog.Logger) *Scheduler {
	return &Scheduler{
		c:   cron.New(cron.WithLogger(cron.DiscardLogger)),
		log: log,
	}
}

// Add registers a job. Empty Spec is a no-op so the caller can pass env
// values directly without a per-job presence check.
func (s *Scheduler) Add(j Job) error {
	if j.Spec == "" {
		s.log.Info("scheduler job disabled (empty spec)", "name", j.Name)
		return nil
	}
	_, err := s.c.AddFunc(j.Spec, func() {
		// Detach from any inherited context so a cron tick doesn't pick
		// up a stale deadline. The job uses context.Background.
		ctx := context.Background()
		s.log.Info("scheduler firing", "name", j.Name)
		if err := j.Run(ctx); err != nil {
			s.log.Error("scheduler job error", "name", j.Name, "err", err)
		}
	})
	if err == nil {
		s.mu.Lock()
		s.jobs = append(s.jobs, j)
		s.mu.Unlock()
	}
	return err
}

// Start kicks off the cron loop. Returns immediately.
func (s *Scheduler) Start() { s.c.Start() }

// Stop blocks until any in-flight jobs finish.
func (s *Scheduler) Stop() { <-s.c.Stop().Done() }

// Jobs returns the registered jobs (for logging/introspection).
func (s *Scheduler) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, len(s.jobs))
	copy(out, s.jobs)
	return out
}
