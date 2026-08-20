// Package chatbot orchestrates chat message persistence and completion.
package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"Metarr/internal/server/chatbot/pagecontext"
	"Metarr/internal/server/chatbot/provider"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
)

// Service splits a chat turn into two calls — CreatePendingMessage and
// StreamMessage — so the model call itself is driven by the stream
// connection (see handlers.ChatStream), not a background goroutine started
// at message-creation time. That split avoids any race between "generation
// starts" and "client subscribes" (no buffering needed to bridge the gap),
// and means a message nobody ever listens for (a closed tab) never burns
// API cost.
type Service struct {
	repo     *mongostore.ChatbotRepo
	registry pagecontext.Registry
	// config returns the live chatbot configuration — a function rather
	// than a captured value, so a provider switch in System > Chatbot takes
	// effect on the very next message without a restart, the same
	// swap-the-singleton pattern appconfig.Get already uses everywhere.
	config func() appconfig.ChatbotConfig
	// selectProvider is provider.Select in production — a field rather
	// than a direct call so tests can substitute a fake Provider and drive
	// StreamMessage (and the WS handler built on it) end to end without a
	// real provider API key.
	selectProvider func(ctx context.Context, cfg appconfig.ChatbotConfig) (provider.Provider, error)
}

// NewService constructs a Service using the real provider.Select.
func NewService(repo *mongostore.ChatbotRepo, registry pagecontext.Registry, config func() appconfig.ChatbotConfig) *Service {
	return NewServiceWithProvider(repo, registry, config, provider.Select)
}

// NewServiceWithProvider is NewService with an injectable provider
// selector — used by tests to drive StreamMessage (and the WS handler
// built on it, handlers.ChatStream) with a fake Provider instead of a real
// one requiring an API key.
func NewServiceWithProvider(
	repo *mongostore.ChatbotRepo,
	registry pagecontext.Registry,
	config func() appconfig.ChatbotConfig,
	selectProvider func(ctx context.Context, cfg appconfig.ChatbotConfig) (provider.Provider, error),
) *Service {
	return &Service{repo: repo, registry: registry, config: config, selectProvider: selectProvider}
}

// CreateMessageRequest is one chat turn from the frontend.
type CreateMessageRequest struct {
	SessionID string
	// PageKey and ContextPayload are empty/nil when the widget has no
	// active page context — the message then carries no extra context.
	PageKey        string
	ContextPayload json.RawMessage
	Text           string
}

// CreateMessageResult is what the frontend needs to light the context icon
// and open the stream connection.
type CreateMessageResult struct {
	MessageID   string
	ContextSent *pagecontext.ContextSentRecord
}

// CreatePendingMessage persists the user's message and a placeholder
// assistant message carrying everything StreamMessage will need to run the
// completion.
func (s *Service) CreatePendingMessage(ctx context.Context, req CreateMessageRequest) (CreateMessageResult, error) {
	var systemText string
	var contextSent *pagecontext.ContextSentRecord
	var tools []provider.ToolSpec

	if req.PageKey != "" {
		if assembler, ok := s.registry[req.PageKey]; ok {
			assembled, err := assembler.Assemble(ctx, req.ContextPayload)
			if err != nil {
				return CreateMessageResult{}, fmt.Errorf("chatbot: assembling %s context: %w", req.PageKey, err)
			}
			systemText = assembled.SystemText
			contextSent = &assembled.Sent
			tools = assembler.Tools()
		}
	}

	// Built from what's already stored, before this turn's user message is
	// inserted — otherwise it would show up twice: once read back from
	// storage, once appended below.
	history, err := s.history(ctx, req.SessionID)
	if err != nil {
		return CreateMessageResult{}, err
	}
	history = append(history, provider.Message{Role: mongostore.ChatRoleUser, Content: req.Text})

	userMessage := &mongostore.ChatMessage{
		SessionID: req.SessionID,
		Role:      mongostore.ChatRoleUser,
		Text:      req.Text,
		Status:    mongostore.ChatStatusComplete,
	}
	if contextSent != nil {
		encoded, err := json.Marshal(contextSent)
		if err != nil {
			return CreateMessageResult{}, fmt.Errorf("chatbot: encoding context-sent record: %w", err)
		}
		userMessage.ContextSent = encoded
	}
	if err := s.repo.Insert(ctx, userMessage); err != nil {
		return CreateMessageResult{}, fmt.Errorf("chatbot: persisting user message: %w", err)
	}

	historyJSON, err := json.Marshal(history)
	if err != nil {
		return CreateMessageResult{}, fmt.Errorf("chatbot: encoding history: %w", err)
	}
	toolsJSON, err := json.Marshal(tools)
	if err != nil {
		return CreateMessageResult{}, fmt.Errorf("chatbot: encoding tools: %w", err)
	}

	assistantMessage := &mongostore.ChatMessage{
		SessionID: req.SessionID,
		Role:      mongostore.ChatRoleAssistant,
		Status:    mongostore.ChatStatusPending,
		PageKey:   req.PageKey,
		System:    systemText,
		History:   historyJSON,
		Tools:     toolsJSON,
	}
	if err := s.repo.Insert(ctx, assistantMessage); err != nil {
		return CreateMessageResult{}, fmt.Errorf("chatbot: persisting assistant placeholder: %w", err)
	}

	return CreateMessageResult{MessageID: assistantMessage.ID.Hex(), ContextSent: contextSent}, nil
}

// history returns every prior message in the session as provider.Message,
// oldest first. A still-pending assistant message (no Text yet — its own
// generation never completed) is skipped: there is nothing to feed back in.
func (s *Service) history(ctx context.Context, sessionID string) ([]provider.Message, error) {
	stored, err := s.repo.ListMessages(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("chatbot: loading history: %w", err)
	}
	messages := make([]provider.Message, 0, len(stored))
	for _, m := range stored {
		if m.Text == "" {
			continue
		}
		messages = append(messages, provider.Message{Role: m.Role, Content: m.Text})
	}
	return messages, nil
}

// StreamMessage runs the completion for messageID, calling emit for every
// delta the provider produces. This is what actually invokes the model —
// driven by the caller's stream connection (handlers.ChatStream), not a
// background goroutine started earlier. A dropped connection (emit
// returning an error) stops further writes but lets generation run to
// completion, so the stored message still ends up correct even if nobody
// was left to watch it stream.
func (s *Service) StreamMessage(ctx context.Context, messageID string, emit func(provider.Delta) error) error {
	objID, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		return fmt.Errorf("chatbot: invalid message id %q: %w", messageID, err)
	}

	stored, err := s.repo.Get(ctx, objID)
	if err != nil {
		return fmt.Errorf("chatbot: loading message %s: %w", messageID, err)
	}
	if stored.Status != mongostore.ChatStatusPending {
		return fmt.Errorf("chatbot: message %s is not pending (status %q)", messageID, stored.Status)
	}

	var history []provider.Message
	if err := json.Unmarshal(stored.History, &history); err != nil {
		return fmt.Errorf("chatbot: decoding stored history: %w", err)
	}
	var tools []provider.ToolSpec
	if len(stored.Tools) > 0 {
		if err := json.Unmarshal(stored.Tools, &tools); err != nil {
			return fmt.Errorf("chatbot: decoding stored tools: %w", err)
		}
	}

	chosenProvider, err := s.selectProvider(ctx, s.config())
	if err != nil {
		return s.finalize(ctx, objID, "", nil, err)
	}

	var finalText strings.Builder
	var finalToolCall *provider.ToolCall
	var streamErr error
	var connDead bool

	err = chosenProvider.Stream(ctx, provider.CompletionRequest{
		Messages: history,
		System:   stored.System,
		Tools:    tools,
	}, func(delta provider.Delta) {
		if delta.Text != "" {
			finalText.WriteString(delta.Text)
		}
		if delta.ToolCall != nil {
			finalToolCall = delta.ToolCall
		}
		if delta.Err != nil {
			streamErr = delta.Err
		}
		if connDead {
			return
		}
		if emitErr := emit(delta); emitErr != nil {
			connDead = true
		}
	})
	if err != nil && streamErr == nil {
		streamErr = err
	}

	return s.finalize(ctx, objID, finalText.String(), finalToolCall, streamErr)
}

func (s *Service) finalize(ctx context.Context, id bson.ObjectID, text string, toolCall *provider.ToolCall, streamErr error) error {
	var toolCallJSON []byte
	if toolCall != nil {
		encoded, err := json.Marshal(toolCall)
		if err != nil {
			return fmt.Errorf("chatbot: encoding tool call: %w", err)
		}
		toolCallJSON = encoded
	}

	status := mongostore.ChatStatusComplete
	if streamErr != nil {
		status = mongostore.ChatStatusFailed
	}
	if err := s.repo.MarkComplete(ctx, id, status, text, toolCallJSON); err != nil {
		return fmt.Errorf("chatbot: marking message %s complete: %w", id.Hex(), err)
	}
	return streamErr
}
