package handlers

import (
	"encoding/json"
	"net/http"

	"Metarr/internal/server/chatbot"
	"Metarr/internal/server/chatbot/pagecontext"
)

// postChatMessageRequest is the body PostChatMessage accepts.
type postChatMessageRequest struct {
	SessionID string          `json:"session_id"`
	PageKey   string          `json:"page_key,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
	Text      string          `json:"text"`
}

// postChatMessageResponse is returned immediately, before generation has
// run — the frontend lights the context icon and opens the stream
// connection (GET /api/chatbot/stream/{message_id}) from this alone.
type postChatMessageResponse struct {
	MessageID   string                         `json:"message_id"`
	ContextSent *pagecontext.ContextSentRecord `json:"context_sent,omitempty"`
}

// PostChatMessage handles POST /api/chatbot/messages. It persists the
// message and a placeholder for the reply, but does not itself run the
// model — that happens when the client connects to the stream endpoint
// this response points at (see ChatStream), so a message nobody ever
// listens for never burns API cost.
//
// @Summary		Send a chat message
// @Description	Persists a chat message (and, on a page with an active context, what was sent). Returns immediately with the assistant message's id — connect to GET /api/chatbot/stream/{message_id} to actually run and stream the reply.
// @Tags			Chatbot
// @Accept			json
// @Produce		json
// @Param			request	body		postChatMessageRequest	true	"Chat message"
// @Success		202		{object}	postChatMessageResponse
// @Failure		400		{string}	string	"invalid request body"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/chatbot/messages [post]
func (h *Handlers) PostChatMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var body postChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.SessionID == "" || body.Text == "" {
		http.Error(w, "session_id and text are required", http.StatusBadRequest)
		return
	}

	result, err := h.ChatbotService.CreatePendingMessage(ctx, chatbot.CreateMessageRequest{
		SessionID:      body.SessionID,
		PageKey:        body.PageKey,
		ContextPayload: body.Context,
		Text:           body.Text,
	})
	if err != nil {
		h.Logger.Error("failed to create chat message", "error", err)
		http.Error(w, "failed to create chat message", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(postChatMessageResponse{
		MessageID:   result.MessageID,
		ContextSent: result.ContextSent,
	}); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// ListChatSessions handles GET /api/chatbot/sessions.
//
// @Summary		List chat sessions
// @Description	Every session that has at least one message, most-recently-active first.
// @Tags			Chatbot
// @Produce		json
// @Success		200	{array}	mongostore.SessionSummary
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/chatbot/sessions [get]
func (h *Handlers) ListChatSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.ChatbotRepo.ListSessions(r.Context())
	if err != nil {
		h.Logger.Error("failed to list chat sessions", "error", err)
		http.Error(w, "failed to list chat sessions", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}

// ListChatMessages handles GET /api/chatbot/sessions/{id}/messages.
//
// @Summary		List one session's messages
// @Description	Every message in a session, oldest first.
// @Tags			Chatbot
// @Produce		json
// @Param			id	path	string	true	"Session id"
// @Success		200	{array}	mongostore.ChatMessage
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/chatbot/sessions/{id}/messages [get]
func (h *Handlers) ListChatMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	messages, err := h.ChatbotRepo.ListMessages(r.Context(), sessionID)
	if err != nil {
		h.Logger.Error("failed to list chat messages", "session_id", sessionID, "error", err)
		http.Error(w, "failed to list chat messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(messages); err != nil {
		h.Logger.Debug("failed to write response body", "error", err)
	}
}
