package provider

import (
	"context"
	"encoding/json"
	"net/http"

	"google.golang.org/genai"

	"Metarr/internal/shared/appconfig"
)

// geminiProvider adapts the Google Gemini API to Provider.
type geminiProvider struct {
	client *genai.Client
	model  string
}

func newGeminiProvider(ctx context.Context, cfg appconfig.ChatbotGeminiConfig, httpClient *http.Client) (Provider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, err
	}
	return &geminiProvider{client: client, model: cfg.Model}, nil
}

func (p *geminiProvider) Stream(ctx context.Context, req CompletionRequest, emit func(Delta)) error {
	contents := make([]*genai.Content, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := genai.RoleUser
		if m.Role == "assistant" {
			role = genai.RoleModel
		}
		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: []*genai.Part{genai.NewPartFromText(m.Content)},
		})
	}

	config := &genai.GenerateContentConfig{}
	if req.System != "" {
		config.SystemInstruction = &genai.Content{Parts: []*genai.Part{genai.NewPartFromText(req.System)}}
	}
	for _, t := range req.Tools {
		var schema genai.Schema
		if err := json.Unmarshal(t.JSONSchema, &schema); err != nil {
			return err
		}
		config.Tools = append(config.Tools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  &schema,
			}},
		})
	}

	for chunk, err := range p.client.Models.GenerateContentStream(ctx, p.model, contents, config) {
		if err != nil {
			emit(Delta{Done: true, Err: err})
			return err
		}
		if len(chunk.Candidates) == 0 || chunk.Candidates[0].Content == nil {
			continue
		}
		for _, part := range chunk.Candidates[0].Content.Parts {
			if part.Text != "" {
				emit(Delta{Text: part.Text})
			}
			if part.FunctionCall != nil {
				args, marshalErr := json.Marshal(part.FunctionCall.Args)
				if marshalErr != nil {
					return marshalErr
				}
				emit(Delta{ToolCall: &ToolCall{Name: part.FunctionCall.Name, Arguments: args}})
			}
		}
	}
	emit(Delta{Done: true})
	return nil
}
