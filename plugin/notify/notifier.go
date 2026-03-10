// Package notify provides a runtime-to-model notification system for ADK agents.
//
// External goroutines call Send() to queue notifications. An ADK plugin drains
// the queue in BeforeModel and injects them into the LLM request.
package notify

import (
	"sync"
	"time"
)

// Kind determines how a notification is delivered to the model.
type Kind int

const (
	// Ephemeral notifications are injected into req.Contents as user-role
	// content. They are not stored in the session — one-off alerts.
	Ephemeral Kind = iota
	// Steering notifications are appended to req.Config.SystemInstruction.
	// They are not stored — silent directives the model follows.
	Steering
)

// Notification is a message from the runtime to the model.
type Notification struct {
	Kind   Kind
	Author string // e.g. "system", "pipeline", "monitor"
	Text   string
}

// Option configures a Notifier.
type Option func(*Notifier)

// WithMaxBatch sets the maximum number of notifications drained per LLM call.
// Excess notifications are dropped oldest-first.
func WithMaxBatch(n int) Option {
	return func(ntf *Notifier) {
		if n > 0 {
			ntf.maxBatch = n
		}
	}
}

// WithInstruction sets the instruction text prepended to ephemeral
// notifications so the model understands what they are.
func WithInstruction(s string) Option {
	return func(ntf *Notifier) {
		ntf.instruction = s
	}
}

// WithRule adds a rule that is checked on every drain.
func WithRule(r Rule) Option {
	return func(ntf *Notifier) {
		ntf.rules = append(ntf.rules, r)
	}
}

const defaultMaxBatch = 20

// Notifier is the notification hub. Thread-safe.
type Notifier struct {
	mu          sync.Mutex
	pending     []Notification
	maxBatch    int
	instruction string
	rules       []Rule
	turnCount   int
	lastActive  time.Time
}

// New creates a Notifier with the given options.
func New(opts ...Option) *Notifier {
	n := &Notifier{
		maxBatch:   defaultMaxBatch,
		lastActive: time.Now(),
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// Send queues a notification for delivery on the next LLM call.
// Safe to call from any goroutine.
func (n *Notifier) Send(note Notification) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending = append(n.pending, note)
}

// drain atomically reads and clears pending notifications, fires rules,
// and enforces maxBatch. Returns nil if nothing to deliver.
func (n *Notifier) drain() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.turnCount++
	now := time.Now()

	// Fire rules.
	rctx := RuleContext{
		TurnCount:    n.turnCount,
		LastActivity: n.lastActive,
	}
	for _, r := range n.rules {
		if note := r.Check(rctx); note != nil {
			n.pending = append(n.pending, *note)
		}
	}

	n.lastActive = now

	if len(n.pending) == 0 {
		return nil
	}

	// Enforce maxBatch — drop oldest.
	result := n.pending
	if len(result) > n.maxBatch {
		result = result[len(result)-n.maxBatch:]
	}
	n.pending = nil
	return result
}
