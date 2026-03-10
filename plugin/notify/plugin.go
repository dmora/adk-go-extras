package notify

import (
	"fmt"
	"strings"

	adkagent "google.golang.org/adk/agent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/genai"
)

// Plugin returns an ADK plugin that drains queued notifications in BeforeModel
// and injects them into the LLM request.
func (n *Notifier) Plugin() *plugin.Plugin {
	p, _ := plugin.New(plugin.Config{
		Name:                "notify",
		BeforeModelCallback: n.beforeModel,
	})
	return p
}

// beforeModel drains pending notifications and routes them by Kind.
func (n *Notifier) beforeModel(_ adkagent.CallbackContext, req *adkmodel.LLMRequest) (*adkmodel.LLMResponse, error) {
	notes := n.drain()
	if len(notes) == 0 {
		return nil, nil
	}

	var ephemeral []Notification
	var steering []Notification
	for _, note := range notes {
		switch note.Kind {
		case Steering:
			steering = append(steering, note)
		default:
			ephemeral = append(ephemeral, note)
		}
	}

	if len(ephemeral) > 0 {
		n.injectEphemeral(req, ephemeral)
	}
	if len(steering) > 0 {
		n.injectSteering(req, steering)
	}
	return nil, nil
}

// injectEphemeral appends notifications as user-role content in req.Contents.
func (n *Notifier) injectEphemeral(req *adkmodel.LLMRequest, notes []Notification) {
	var b strings.Builder
	if n.instruction != "" {
		b.WriteString(n.instruction)
		b.WriteString("\n")
	}
	for _, note := range notes {
		fmt.Fprintf(&b, "[%s] %s\n", note.Author, note.Text)
	}
	req.Contents = append(req.Contents, &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{genai.NewPartFromText(b.String())},
	})
}

// injectSteering appends notifications to the system instruction.
func (n *Notifier) injectSteering(req *adkmodel.LLMRequest, notes []Notification) {
	var b strings.Builder
	b.WriteString("\n<runtime_notification>\n")
	for _, note := range notes {
		fmt.Fprintf(&b, "[%s] %s\n", note.Author, note.Text)
	}
	b.WriteString("</runtime_notification>")

	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}
	if req.Config.SystemInstruction == nil {
		req.Config.SystemInstruction = &genai.Content{
			Role:  "user",
			Parts: []*genai.Part{},
		}
	}
	req.Config.SystemInstruction.Parts = append(
		req.Config.SystemInstruction.Parts,
		genai.NewPartFromText(b.String()),
	)
}
