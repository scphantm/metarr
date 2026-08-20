package mongostore

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// connectTestChatbotRepo opens a ChatbotRepo against a real MongoDB
// instance, same pattern as connectTestWorkflowRepo — skips cleanly when no
// MongoDB is reachable, and only ever deletes the specific session this
// test created.
func connectTestChatbotRepo(t *testing.T) (repo *ChatbotRepo, cleanupSession func(sessionID string)) {
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

	repo = NewChatbotRepo(client, "metarr")
	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}

	t.Cleanup(func() {
		disconnectCtx, disconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer disconnectCancel()
		_ = client.Disconnect(disconnectCtx)
	})

	cleanupSession = func(sessionID string) {
		t.Cleanup(func() {
			deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer deleteCancel()
			_, _ = client.Database("metarr").Collection(chatbotMessageCollection).
				DeleteMany(deleteCtx, bson.M{"session_id": sessionID})
		})
	}
	return repo, cleanupSession
}

func TestChatbotRepoRoundTrip(t *testing.T) {
	repo, cleanupSession := connectTestChatbotRepo(t)
	ctx := context.Background()
	sessionID := "test-session-" + bson.NewObjectID().Hex()
	cleanupSession(sessionID)

	user := &ChatMessage{
		SessionID:   sessionID,
		Role:        ChatRoleUser,
		Text:        "hello",
		Status:      ChatStatusComplete,
		ContextSent: []byte(`{"page_key":"workflow","items":[]}`),
	}
	if err := repo.Insert(ctx, user); err != nil {
		t.Fatalf("Insert(user) error = %v", err)
	}
	if user.ID.IsZero() {
		t.Fatal("Insert(user) did not assign an ID")
	}

	assistant := &ChatMessage{
		SessionID: sessionID,
		Role:      ChatRoleAssistant,
		Status:    ChatStatusPending,
		PageKey:   "workflow",
		System:    "system prompt",
		History:   []byte(`[{"role":"user","content":"hello"}]`),
		Tools:     []byte(`[]`),
	}
	if err := repo.Insert(ctx, assistant); err != nil {
		t.Fatalf("Insert(assistant) error = %v", err)
	}

	fetched, err := repo.Get(ctx, assistant.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if fetched.Status != ChatStatusPending || fetched.System != "system prompt" {
		t.Errorf("Get() = %+v, want pending with generation input intact", fetched)
	}

	if err := repo.MarkComplete(ctx, assistant.ID, ChatStatusComplete, "the reply", []byte(`{"name":"propose_workflow_edit"}`)); err != nil {
		t.Fatalf("MarkComplete() error = %v", err)
	}

	finalized, err := repo.Get(ctx, assistant.ID)
	if err != nil {
		t.Fatalf("Get() after MarkComplete error = %v", err)
	}
	if finalized.Status != ChatStatusComplete {
		t.Errorf("Status = %q, want %q", finalized.Status, ChatStatusComplete)
	}
	if finalized.Text != "the reply" {
		t.Errorf("Text = %q, want %q", finalized.Text, "the reply")
	}
	if len(finalized.System) != 0 || len(finalized.History) != 0 || len(finalized.Tools) != 0 || finalized.PageKey != "" {
		t.Errorf("MarkComplete left generation input behind: %+v", finalized)
	}
	if len(finalized.ToolCall) == 0 {
		t.Error("ToolCall was not persisted")
	}

	messages, err := repo.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(messages) != 2 || messages[0].Role != ChatRoleUser || messages[1].Role != ChatRoleAssistant {
		t.Fatalf("ListMessages() = %+v, want [user, assistant] in order", messages)
	}

	sessions, err := repo.ListSessions(ctx)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.SessionID == sessionID {
			found = true
		}
	}
	if !found {
		t.Error("ListSessions did not include this test's session")
	}
}

func TestChatbotRepoGetMissingReturnsErrNotFound(t *testing.T) {
	repo, _ := connectTestChatbotRepo(t)

	_, err := repo.Get(context.Background(), bson.NewObjectID())
	if err != ErrNotFound {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
}
