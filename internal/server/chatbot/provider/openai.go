package provider

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"Metarr/internal/shared/appconfig"
)

// openaiProvider adapts the OpenAI Chat Completions API to Provider.
type openaiProvider struct {
	client openai.Client
	model  string
}

func newOpenAIProvider(cfg appconfig.ChatbotOpenAIConfig, httpClient *http.Client) Provider {
	return &openaiProvider{
		client: openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithHTTPClient(httpClient)),
		model:  cfg.Model,
	}
}

func (p *openaiProvider) Stream(ctx context.Context, req CompletionRequest, emit func(Delta)) error {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages)+1)
	if req.System != "" {
		messages = append(messages, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		if m.Role == "assistant" {
			messages = append(messages, openai.AssistantMessage(m.Content))
		} else {
			messages = append(messages, openai.UserMessage(m.Content))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(p.model),
		Messages: messages,
	}
	for _, t := range req.Tools {
		var schema map[string]any
		if err := json.Unmarshal(t.JSONSchema, &schema); err != nil {
			return err
		}
		params.Tools = append(params.Tools, openai.ChatCompletionFunctionTool(
			openai.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  schema,
			},
		))
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	// OpenAI streams tool-call argument fragments indexed by position, so a
	// single in-flight call is accumulated by index rather than assumed to
	// be the only one.
	type pendingCall struct {
		name string
		args string
	}
	pending := map[int64]*pendingCall{}

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != "" {
			emit(Delta{Text: delta.Content})
		}
		for _, tc := range delta.ToolCalls {
			call, ok := pending[tc.Index]
			if !ok {
				call = &pendingCall{}
				pending[tc.Index] = call
			}
			if tc.Function.Name != "" {
				call.name = tc.Function.Name
			}
			call.args += tc.Function.Arguments
		}
	}
	if err := stream.Err(); err != nil {
		emit(Delta{Done: true, Err: err})
		return err
	}
	for _, call := range pending {
		emit(Delta{ToolCall: &ToolCall{Name: call.name, Arguments: json.RawMessage(call.args)}})
	}
	emit(Delta{Done: true})
	return nil
}
