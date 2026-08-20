package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"Metarr/internal/shared/appconfig"
)

// claudeProvider adapts the Anthropic Messages API to Provider.
type claudeProvider struct {
	client anthropic.Client
	model  string
}

func newClaudeProvider(cfg appconfig.ChatbotClaudeConfig, httpClient *http.Client) Provider {
	return &claudeProvider{
		client: anthropic.NewClient(option.WithAPIKey(cfg.APIKey), option.WithHTTPClient(httpClient)),
		model:  cfg.Model,
	}
}

func (p *claudeProvider) Stream(ctx context.Context, req CompletionRequest, emit func(Delta)) error {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: req.System}},
		Messages:  make([]anthropic.MessageParam, 0, len(req.Messages)),
		Tools:     make([]anthropic.ToolUnionParam, 0, len(req.Tools)),
	}
	for _, m := range req.Messages {
		block := anthropic.NewTextBlock(m.Content)
		if m.Role == "assistant" {
			params.Messages = append(params.Messages, anthropic.NewAssistantMessage(block))
		} else {
			params.Messages = append(params.Messages, anthropic.NewUserMessage(block))
		}
	}
	for _, t := range req.Tools {
		var schema map[string]any
		if err := json.Unmarshal(t.JSONSchema, &schema); err != nil {
			return fmt.Errorf("tool %q has invalid JSON schema: %w", t.Name, err)
		}
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: schema["properties"],
				},
			},
		})
	}

	stream := p.client.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var toolName string
	var toolArgs string
	for stream.Next() {
		event := stream.Current()
		switch delta := event.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			if block, ok := delta.ContentBlock.AsAny().(anthropic.ToolUseBlock); ok {
				toolName = block.Name
				toolArgs = ""
			}
		case anthropic.ContentBlockDeltaEvent:
			switch d := delta.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				emit(Delta{Text: d.Text})
			case anthropic.InputJSONDelta:
				toolArgs += d.PartialJSON
			}
		case anthropic.ContentBlockStopEvent:
			if toolName != "" {
				emit(Delta{ToolCall: &ToolCall{Name: toolName, Arguments: json.RawMessage(toolArgs)}})
				toolName = ""
			}
		}
	}
	if err := stream.Err(); err != nil {
		emit(Delta{Done: true, Err: err})
		return err
	}
	emit(Delta{Done: true})
	return nil
}
