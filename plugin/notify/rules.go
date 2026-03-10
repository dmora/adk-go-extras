package notify

import "time"

// Rule is a trigger that produces notifications based on runtime context.
// Rules are checked on every drain (once per BeforeModel call).
type Rule interface {
	Check(ctx RuleContext) *Notification
}

// RuleContext is the state available to rules at drain time.
type RuleContext struct {
	TurnCount    int
	LastActivity time.Time
}

// everyNTurns fires a notification every N turns.
type everyNTurns struct {
	n    int
	note Notification
}

// EveryNTurns returns a rule that fires the given notification every n turns.
func EveryNTurns(n int, note Notification) Rule {
	return &everyNTurns{n: n, note: note}
}

func (r *everyNTurns) Check(ctx RuleContext) *Notification {
	if r.n <= 0 {
		return nil
	}
	if ctx.TurnCount%r.n == 0 {
		return &r.note
	}
	return nil
}

// afterInactivity fires a notification when the time since last activity
// exceeds the given duration.
type afterInactivity struct {
	d    time.Duration
	note Notification
}

// AfterInactivity returns a rule that fires when the agent has been inactive
// for at least d since the last drain.
func AfterInactivity(d time.Duration, note Notification) Rule {
	return &afterInactivity{d: d, note: note}
}

func (r *afterInactivity) Check(ctx RuleContext) *Notification {
	if time.Since(ctx.LastActivity) >= r.d {
		return &r.note
	}
	return nil
}
