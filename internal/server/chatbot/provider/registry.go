package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"Metarr/internal/shared/appconfig"
)

// completionHTTPTimeout bounds a single provider HTTP call. Generous
// because completions (especially with tool use) can legitimately take a
// while, but still finite so a wedged upstream can't hang a chat forever.
const completionHTTPTimeout = 2 * time.Minute

// Select constructs the Provider matching cfg.Provider — the only place in
// the chatbot feature that interprets the discriminator. Completion calls
// are one-shot and often streamed, so they use a plain client rather than
// the project's cached HTTP client (see CLAUDE.md's General Conventions,
// scoped to metadata-lookup interfaces).
func Select(ctx context.Context, cfg appconfig.ChatbotConfig) (Provider, error) {
	httpClient := &http.Client{Timeout: completionHTTPTimeout}

	switch cfg.Provider {
	case appconfig.ChatbotProviderClaude:
		return newClaudeProvider(cfg.Claude, httpClient), nil
	case appconfig.ChatbotProviderOpenAI:
		return newOpenAIProvider(cfg.OpenAI, httpClient), nil
	case appconfig.ChatbotProviderGemini:
		return newGeminiProvider(ctx, cfg.Gemini, httpClient)
	case appconfig.ChatbotProviderMCP:
		return newMCPProvider(cfg.MCP), nil
	default:
		return nil, fmt.Errorf("unknown chatbot provider %q", cfg.Provider)
	}
}
