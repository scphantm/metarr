package handlers

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"Metarr/internal/server/chatbot/provider"
)

// ChatStream handles GET /api/chatbot/stream/{id}. Unlike wsbus.Hub's
// topics — one shared, ticker-polled value fanned out to many
// subscribers — a chat reply is a private, one-shot stream for the one
// request that asked for it, so it gets its own connection rather than
// riding the hub: connecting is what runs the completion (see
// chatbot.Service.StreamMessage), writing each delta straight to the
// socket as the provider produces it. Authorization already happened in
// the router's protect() wrapper before this handler runs.
//
// @Summary		Stream a chat reply
// @Description	Connecting runs the completion for the given (pending) assistant message id and streams each delta as a JSON frame, ending with one carrying done:true. A message that isn't pending (already streamed, or never created) closes the connection immediately with an error frame.
// @Tags			Chatbot
// @Param			id	path	string	true	"Assistant message id, from POST /api/chatbot/messages's response"
// @Security		ApiKeyHeaderAuth
// @Security		ApiKeyQueryAuth
// @Router			/api/chatbot/stream/{id} [get]
func (h *Handlers) ChatStream(w http.ResponseWriter, r *http.Request) {
	messageID := r.PathValue("id")

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// See wsbus.Hub.ServeHTTP's identical comment: this API has no
		// ambient credential a foreign origin could ride, and the dev
		// setup's Vite-proxies-to-API split means the two origins
		// legitimately disagree.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		h.Logger.Warn("chat stream websocket upgrade failed", "error", err)
		return
	}
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()

	ctx := r.Context()
	streamErr := h.ChatbotService.StreamMessage(ctx, messageID, func(delta provider.Delta) error {
		return wsjson.Write(ctx, ws, delta)
	})
	if streamErr != nil {
		h.Logger.Warn("chat stream ended with an error", "message_id", messageID, "error", streamErr)
		_ = wsjson.Write(ctx, ws, provider.Delta{Done: true, Err: streamErr})
	}
}
