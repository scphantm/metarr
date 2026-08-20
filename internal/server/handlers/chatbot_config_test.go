package handlers

import (
	"testing"

	"Metarr/internal/shared/appconfig"
)

func TestValidateChatbotProviderAcceptsEachKnownProvider(t *testing.T) {
	for _, provider := range []string{
		appconfig.ChatbotProviderClaude,
		appconfig.ChatbotProviderOpenAI,
		appconfig.ChatbotProviderGemini,
		appconfig.ChatbotProviderMCP,
	} {
		if err := validateChatbotProvider(provider); err != nil {
			t.Errorf("validateChatbotProvider(%q) = %v; want accepted", provider, err)
		}
	}
}

func TestValidateChatbotProviderRejectsUnknownOrEmpty(t *testing.T) {
	for _, provider := range []string{"", "local", "chatgpt", "Claude"} {
		if err := validateChatbotProvider(provider); err == nil {
			t.Errorf("validateChatbotProvider(%q) accepted; want rejected", provider)
		}
	}
}
