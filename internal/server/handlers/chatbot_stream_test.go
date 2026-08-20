package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"Metarr/internal/server/chatbot"
	"Metarr/internal/server/chatbot/pagecontext"
	"Metarr/internal/server/chatbot/provider"
	"Metarr/internal/server/mongostore"
	"Metarr/internal/shared/appconfig"
)

// connectTestChatbotRepo opens a ChatbotRepo against a real MongoDB
// instance, same skip-cleanly-if-unreachable pattern as
// mongostore.connectTestWorkflowRepo. It returns a fresh, random session id
// already scoped for cleanup, since every test here only needs one session.
func connectTestChatbotRepo(t *testing.T) (*mongostore.ChatbotRepo, string) {
	t.Helper()

	uri := os.Getenv("METARR_TEST_MONGO_URI")
	if uri == "" {
		uri = "mongodb://metarr:metarr@localhost:27017/metarr?authSource=metarr"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("mongo.Connect: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("no reachable MongoDB at %s: %v", uri, err)
	}

	repo := mongostore.NewChatbotRepo(client, "metarr")
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}

	sessionID := "test-stream-" + bson.NewObjectID().Hex()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = client.Database("metarr").Collection("chatbot_messages").
			DeleteMany(cleanupCtx, bson.M{"session_id": sessionID})
		_ = client.Disconnect(cleanupCtx)
	})
	return repo, sessionID
}

// fakeProvider is a provider.Provider test double — no network, no API key,
// just a scripted sequence of deltas, so the WS transport can be verified
// end to end without a real model call.
type fakeProvider struct {
	deltas []provider.Delta
}

func (f fakeProvider) Stream(_ context.Context, _ provider.CompletionRequest, emit func(provider.Delta)) error {
	for _, d := range f.deltas {
		emit(d)
	}
	return nil
}

func serveChatStream(t *testing.T, h *Handlers) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/chatbot/stream/{id}", h.ChatStream)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// readFrame reads and decodes a single JSON delta frame off conn.
func readFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) (text string, done bool, errText string) {
	t.Helper()

	var frame struct {
		Text string `json:"text"`
		Done bool   `json:"done"`
		Err  string `json:"error"`
	}
	if err := wsjson.Read(ctx, conn, &frame); err != nil {
		t.Fatalf("reading frame: %v", err)
	}
	return frame.Text, frame.Done, frame.Err
}

// TestChatStreamDeliversDeltasInOrderAndPersistsTheFinalMessage drives the
// whole path: CreatePendingMessage (called directly, not over HTTP — this
// test's focus is the stream transport) creates the pending assistant
// message, then a real WebSocket client connects to handlers.ChatStream,
// which is what actually invokes the fake provider and streams its deltas
// back — confirming connecting is what runs the completion, not a
// background goroutine started earlier.
func TestChatStreamDeliversDeltasInOrderAndPersistsTheFinalMessage(t *testing.T) {
	repo, sessionID := connectTestChatbotRepo(t)
	fake := fakeProvider{deltas: []provider.Delta{
		{Text: "Hello "},
		{Text: "world"},
		{Done: true},
	}}
	service := chatbot.NewServiceWithProvider(
		repo,
		pagecontext.Registry{},
		func() appconfig.ChatbotConfig { return appconfig.ChatbotConfig{} },
		func(context.Context, appconfig.ChatbotConfig) (provider.Provider, error) { return fake, nil },
	)
	h := &Handlers{ChatbotRepo: repo, ChatbotService: service, Logger: discardLogger()}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := service.CreatePendingMessage(ctx, chatbot.CreateMessageRequest{SessionID: sessionID, Text: "hi"})
	if err != nil {
		t.Fatalf("CreatePendingMessage() error = %v", err)
	}

	conn, _, err := websocket.Dial(ctx, serveChatStream(t, h)+"/api/chatbot/stream/"+created.MessageID, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	text1, done1, _ := readFrame(t, ctx, conn)
	text2, done2, _ := readFrame(t, ctx, conn)
	_, done3, _ := readFrame(t, ctx, conn)

	if text1 != "Hello " || done1 {
		t.Errorf("frame 1 = (%q, done=%v), want (%q, done=false)", text1, done1, "Hello ")
	}
	if text2 != "world" || done2 {
		t.Errorf("frame 2 = (%q, done=%v), want (%q, done=false)", text2, done2, "world")
	}
	if !done3 {
		t.Error("frame 3 was not the final done frame")
	}

	msgID, err := bson.ObjectIDFromHex(created.MessageID)
	if err != nil {
		t.Fatalf("parsing message id: %v", err)
	}
	stored, err := repo.Get(context.Background(), msgID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status != mongostore.ChatStatusComplete {
		t.Errorf("Status = %q, want %q", stored.Status, mongostore.ChatStatusComplete)
	}
	if stored.Text != "Hello world" {
		t.Errorf("Text = %q, want %q", stored.Text, "Hello world")
	}
}

// TestChatStreamSurfacesAMidStreamProviderError confirms a provider failure
// reaches the client as an error frame rather than a hung or silently
// closed connection, and that the stored message ends up marked failed
// rather than complete.
func TestChatStreamSurfacesAMidStreamProviderError(t *testing.T) {
	repo, sessionID := connectTestChatbotRepo(t)
	failure := errors.New("upstream exploded")
	service := chatbot.NewServiceWithProvider(
		repo,
		pagecontext.Registry{},
		func() appconfig.ChatbotConfig { return appconfig.ChatbotConfig{} },
		func(context.Context, appconfig.ChatbotConfig) (provider.Provider, error) { return nil, failure },
	)
	h := &Handlers{ChatbotRepo: repo, ChatbotService: service, Logger: discardLogger()}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := service.CreatePendingMessage(ctx, chatbot.CreateMessageRequest{SessionID: sessionID, Text: "hi"})
	if err != nil {
		t.Fatalf("CreatePendingMessage() error = %v", err)
	}

	conn, _, err := websocket.Dial(ctx, serveChatStream(t, h)+"/api/chatbot/stream/"+created.MessageID, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	_, done, errText := readFrame(t, ctx, conn)
	if !done || errText == "" {
		t.Errorf("frame = (done=%v, error=%q), want (done=true, a non-empty error)", done, errText)
	}

	msgID, err := bson.ObjectIDFromHex(created.MessageID)
	if err != nil {
		t.Fatalf("parsing message id: %v", err)
	}
	stored, err := repo.Get(context.Background(), msgID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if stored.Status != mongostore.ChatStatusFailed {
		t.Errorf("Status = %q, want %q", stored.Status, mongostore.ChatStatusFailed)
	}
}
