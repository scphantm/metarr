package appconfig

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDefaultChatbotConfigHasAValidProvider(t *testing.T) {
	cfg := Default().Chatbot
	switch cfg.Provider {
	case ChatbotProviderClaude, ChatbotProviderOpenAI, ChatbotProviderGemini, ChatbotProviderMCP:
	default:
		t.Errorf("Default().Chatbot.Provider = %q; want one of the four known providers", cfg.Provider)
	}
}

// Every provider's settings must round-trip through the JSON the config API
// sends and receives — this is what lets switching providers keep whatever
// was typed into the other three.
func TestChatbotConfigRoundTripsThroughJSON(t *testing.T) {
	original := ChatbotConfig{
		Enabled:  true,
		Provider: ChatbotProviderMCP,
		Claude:   ChatbotClaudeConfig{APIKey: "sk-claude", Model: "claude-sonnet-4-5"},
		OpenAI:   ChatbotOpenAIConfig{APIKey: "sk-openai", Model: "gpt-5"},
		Gemini:   ChatbotGeminiConfig{APIKey: "sk-gemini", Model: "gemini-2.5-pro"},
		MCP: ChatbotMCPConfig{
			Transport: "http",
			URL:       "https://example.internal/mcp",
			ToolName:  "chat",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var round ChatbotConfig
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(round, original) {
		t.Errorf("round-tripped config = %+v; want %+v", round, original)
	}
}
