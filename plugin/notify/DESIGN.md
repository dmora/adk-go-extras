# plugin/notify — Runtime-to-Model Notification System

## Problem

ADK provides no built-in mechanism for the host application to inject runtime
messages into the model's conversation. When a pipeline completes, a timer
fires, or a resource threshold is breached, there is no clean way to tell the
model about it.

ADK's `ConvertForeignEvent` prefixes foreign-authored events with
`[author] said: ...`, but nothing teaches the model what that means. And there
is no thread-safe API for pushing notifications from arbitrary goroutines into
an active agent loop.

## What This Package Solves

A producer/consumer notification system for any ADK Go agent:

1. **Runtime-to-model communication** — status updates, results, alerts
2. **Periodic reminders** — "after N turns, remind the model to check X"
3. **Inactivity nudges** — "after X time idle, send a nudge"
4. **Critical alerts** — "something happened, notify on next LLM call"
5. **Steering directives** — silent instructions the model follows without
   surfacing to the user

## Architecture

```
Producers (any goroutine)          Consumer (ADK lifecycle)
┌──────────────┐                   ┌──────────────────────┐
│ Pipeline FSM  │──┐               │  BeforeModel:        │
│ Timer/Cron    │──┼── Send() ──→  │    drain queue        │
│ Tool result   │──┤   (mutex)     │    check rules        │
│ Resource mon. │──┘               │    inject into req    │
└──────────────┘                   └──────────────────────┘
```

### Delivery Point

`BeforeModel` fires before EVERY LLM call, including mid-tool-loop. When the
agent is in a `LLM → tool → LLM → tool` cycle, `BeforeModel` fires between
each LLM call. Queued notifications get injected at exactly the right moment.

### Delivery Modes

| Mode | Where injected | Persists? | Use case |
|------|---------------|-----------|----------|
| Ephemeral | `req.Contents` | No | One-off alerts, reminders |
| Steering | `req.Config.SystemInstruction` | No | Silent directives for the model |
| Persistent | Session event via `AppendEvent()` | Yes | Status updates the model should remember |

### Thread Safety

External goroutines call `Send()` which appends to a mutex-protected queue.
`BeforeModel` drains the queue under the same lock. No channel needed — the
drain is synchronous with the LLM call lifecycle.

### Overflow Protection

Max batch size per drain to avoid context window blowout. Notifications beyond
the cap are dropped oldest-first or summarized.

### Idle Agent

If the agent is waiting for user input, notifications queue until the next LLM
call. Lazy delivery is acceptable for v1. Future: a wake mechanism to trigger a
synthetic turn for high-priority notifications.

## Proposed API

```go
package notify

// Kind determines how a notification is delivered.
type Kind int

const (
    Ephemeral  Kind = iota // Injected into conversation, not stored
    Steering               // Appended to system instruction, not stored
    Persistent             // Appended as session event, stored permanently
)

// Notification is a message from the runtime to the model.
type Notification struct {
    Kind   Kind
    Author string // e.g. "system", "pipeline", "monitor"
    Text   string
}

// Notifier is the notification hub. Thread-safe.
// Pass to producers (pipeline, timers) and wire its Plugin() into the runner.
type Notifier struct { ... }

func New(opts ...Option) *Notifier

// Send queues a notification for delivery on the next LLM call.
func (n *Notifier) Send(note Notification)

// Plugin returns an ADK plugin that delivers queued notifications.
func (n *Notifier) Plugin() *plugin.Plugin

// Rule is a trigger that produces notifications based on context.
type Rule interface {
    Check(ctx RuleContext) *Notification
}

// RuleContext is the state available to rules at drain time.
type RuleContext struct {
    TurnCount    int
    LastActivity time.Time
    AgentName    string
}

// Built-in rules:
func EveryNTurns(n int, note Notification) Rule
func AfterInactivity(d time.Duration, note Notification) Rule
```

## Consumer Wiring (Example)

```go
// In the host application:
ntf := notify.New(
    notify.WithMaxBatch(10),
    notify.WithInstruction("Messages prefixed with [system] are runtime notifications, not user input."),
    notify.WithRule(notify.EveryNTurns(5, notify.Notification{
        Kind: notify.Steering, Author: "system",
        Text: "Check context window usage before continuing.",
    })),
)

runner.New(runner.Config{
    PluginConfig: runner.PluginConfig{
        Plugins: []*plugin.Plugin{ntf.Plugin()},
    },
})

// From any goroutine:
ntf.Send(notify.Notification{
    Kind:   notify.Ephemeral,
    Author: "system",
    Text:   "Station DRAFT completed with exit code 0.",
})
```

## Package Layout

```
plugin/notify/
├── notifier.go    # Notifier, Send(), New(), options
├── plugin.go      # ADK plugin: BeforeModel drain + inject
├── rules.go       # Rule interface, EveryNTurns, AfterInactivity
└── notify_test.go # Tests
```

## Dependencies

- `google.golang.org/adk` (plugin, agent, model, session, genai types)
- No other external dependencies

## Open Questions

- Should `Persistent` notifications require the caller to pass session context,
  or should the plugin capture it from `BeforeRun`?
- Should overflow notifications be summarized ("5 notifications dropped") or
  silently discarded?
- Should the instruction text be a required option or have a sensible default?
