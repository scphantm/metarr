package provider

import (
	"context"
	"testing"

	"Metarr/internal/shared/appconfig"
)

func TestSelectReturnsAProviderForEachKnownDiscriminator(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []string{
		appconfig.ChatbotProviderClaude,
		appconfig.ChatbotProviderOpenAI,
		appconfig.ChatbotProviderGemini,
		appconfig.ChatbotProviderMCP,
	} {
		cfg := appconfig.ChatbotConfig{
			Provider: tc,
			// Gemini's client validates an API key is present at
			// construction time, unlike the other three.
			Gemini: appconfig.ChatbotGeminiConfig{APIKey: "fake"},
		}
		got, err := Select(ctx, cfg)
		if err != nil {
			t.Errorf("Select(%q) error = %v; want a Provider", tc, err)
			continue
		}
		if got == nil {
			t.Errorf("Select(%q) = nil Provider", tc)
		}
	}
}

func TestSelectRejectsAnUnknownProvider(t *testing.T) {
	_, err := Select(context.Background(), appconfig.ChatbotConfig{Provider: "local"})
	if err == nil {
		t.Fatal("Select(\"local\") accepted; want rejected now that the fourth provider is \"mcp\"")
	}
}
