package notify

import (
	"strings"
	"testing"
	"time"

	adkmodel "google.golang.org/adk/model"
	"google.golang.org/genai"
)

func TestSendDrainRoundTrip(t *testing.T) {
	n := New()
	n.Send(Notification{Kind: Ephemeral, Author: "system", Text: "hello"})
	n.Send(Notification{Kind: Steering, Author: "pipeline", Text: "steer"})

	notes := n.drain()
	if len(notes) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notes))
	}
	if notes[0].Text != "hello" {
		t.Errorf("expected 'hello', got %q", notes[0].Text)
	}
	if notes[1].Text != "steer" {
		t.Errorf("expected 'steer', got %q", notes[1].Text)
	}
}

func TestDrainIsAtomic(t *testing.T) {
	n := New()
	n.Send(Notification{Kind: Ephemeral, Author: "a", Text: "first"})
	_ = n.drain()

	// Second drain returns nil (only rule-produced notifications, if any).
	n2 := New() // fresh notifier, no rules
	n2.Send(Notification{Kind: Ephemeral, Author: "a", Text: "x"})
	_ = n2.drain()
	notes := n2.drain()
	if notes != nil {
		t.Fatalf("expected nil after second drain, got %d notes", len(notes))
	}
}

func TestMaxBatchOverflow(t *testing.T) {
	n := New(WithMaxBatch(2))
	n.Send(Notification{Kind: Ephemeral, Author: "a", Text: "1"})
	n.Send(Notification{Kind: Ephemeral, Author: "a", Text: "2"})
	n.Send(Notification{Kind: Ephemeral, Author: "a", Text: "3"})

	notes := n.drain()
	if len(notes) != 2 {
		t.Fatalf("expected 2 (maxBatch), got %d", len(notes))
	}
	// Oldest dropped — should have "2" and "3".
	if notes[0].Text != "2" || notes[1].Text != "3" {
		t.Errorf("expected oldest dropped, got %q and %q", notes[0].Text, notes[1].Text)
	}
}

func TestEveryNTurnsRule(t *testing.T) {
	note := Notification{Kind: Steering, Author: "system", Text: "check"}
	n := New(WithRule(EveryNTurns(3, note)))

	// Turns 1, 2 — no rule fire.
	for i := 0; i < 2; i++ {
		notes := n.drain()
		if notes != nil {
			t.Fatalf("turn %d: expected no notes, got %d", i+1, len(notes))
		}
	}
	// Turn 3 — rule fires.
	notes := n.drain()
	if len(notes) != 1 {
		t.Fatalf("turn 3: expected 1 note, got %d", len(notes))
	}
	if notes[0].Text != "check" {
		t.Errorf("expected 'check', got %q", notes[0].Text)
	}
}

func TestAfterInactivityRule(t *testing.T) {
	note := Notification{Kind: Ephemeral, Author: "monitor", Text: "idle"}
	n := New(WithRule(AfterInactivity(10*time.Millisecond, note)))

	// First drain — lastActivity is "now", so rule shouldn't fire.
	// But lastActive was set at New() time, so we need to ensure it's recent.
	n.lastActive = time.Now()
	notes := n.drain()
	if notes != nil {
		t.Fatalf("expected no notes immediately, got %d", len(notes))
	}

	// Wait for inactivity threshold.
	time.Sleep(15 * time.Millisecond)
	notes = n.drain()
	if len(notes) != 1 {
		t.Fatalf("expected 1 note after inactivity, got %d", len(notes))
	}
	if notes[0].Text != "idle" {
		t.Errorf("expected 'idle', got %q", notes[0].Text)
	}
}

func TestBeforeModelEphemeral(t *testing.T) {
	n := New(WithInstruction("These are runtime notifications."))
	n.Send(Notification{Kind: Ephemeral, Author: "system", Text: "station done"})

	req := &adkmodel.LLMRequest{}
	_, err := n.beforeModel(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(req.Contents))
	}
	content := req.Contents[0]
	if content.Role != genai.RoleUser {
		t.Errorf("expected user role, got %q", content.Role)
	}
	text := content.Parts[0].Text
	if !strings.Contains(text, "These are runtime notifications.") {
		t.Error("expected instruction in text")
	}
	if !strings.Contains(text, "[system] station done") {
		t.Error("expected notification in text")
	}
}

func TestBeforeModelSteering(t *testing.T) {
	n := New()
	n.Send(Notification{Kind: Steering, Author: "pipeline", Text: "check context"})

	req := &adkmodel.LLMRequest{
		Config: &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{genai.NewPartFromText("base prompt")},
			},
		},
	}
	_, err := n.beforeModel(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Config.SystemInstruction.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(req.Config.SystemInstruction.Parts))
	}
	text := req.Config.SystemInstruction.Parts[1].Text
	if !strings.Contains(text, "<runtime_notification>") {
		t.Error("expected runtime_notification tag")
	}
	if !strings.Contains(text, "[pipeline] check context") {
		t.Error("expected notification text")
	}
}

func TestBeforeModelMixedRouting(t *testing.T) {
	n := New()
	n.Send(Notification{Kind: Ephemeral, Author: "sys", Text: "alert"})
	n.Send(Notification{Kind: Steering, Author: "pipe", Text: "directive"})

	req := &adkmodel.LLMRequest{}
	_, err := n.beforeModel(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ephemeral goes to Contents.
	if len(req.Contents) != 1 {
		t.Fatalf("expected 1 content for ephemeral, got %d", len(req.Contents))
	}
	if !strings.Contains(req.Contents[0].Parts[0].Text, "[sys] alert") {
		t.Error("expected ephemeral content")
	}

	// Steering goes to SystemInstruction.
	if req.Config == nil || req.Config.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction to be set")
	}
	if !strings.Contains(req.Config.SystemInstruction.Parts[0].Text, "[pipe] directive") {
		t.Error("expected steering content")
	}
}

func TestBeforeModelNoNotifications(t *testing.T) {
	n := New()

	req := &adkmodel.LLMRequest{}
	_, err := n.beforeModel(nil, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Nothing should be modified (only turn count incremented).
	if len(req.Contents) != 0 {
		t.Errorf("expected no contents, got %d", len(req.Contents))
	}
}
