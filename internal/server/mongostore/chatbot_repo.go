package mongostore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const chatbotMessageCollection = "chatbot_messages"

// The two roles a ChatMessage can carry.
const (
	ChatRoleUser      = "user"
	ChatRoleAssistant = "assistant"
)

// A ChatMessage's lifecycle: an assistant message starts pending (created
// alongside its user message, before generation has run), and ends either
// complete or failed once the stream handler has actually run the
// completion. User messages are always complete — they need no generation.
const (
	ChatStatusComplete = "complete"
	ChatStatusPending  = "pending"
	ChatStatusFailed   = "failed"
)

// ChatMessage is one turn of a chat session. Everything the chatbot package
// puts real types around — the context-sent record, a proposed tool call,
// the conversation history and tools a pending generation needs — is stored
// here as opaque JSON rather than typed fields, so this package never
// imports the chatbot feature's own packages; the chatbot service layer
// owns encoding and decoding them.
type ChatMessage struct {
	ID        bson.ObjectID `bson:"_id,omitempty" json:"id"`
	SessionID string        `bson:"session_id" json:"session_id"`
	Role      string        `bson:"role" json:"role"`
	Text      string        `bson:"text,omitempty" json:"text,omitempty"`
	Status    string        `bson:"status" json:"status"`

	// ContextSent is set only on a user message that had an active page
	// context at send time — JSON-encoded pagecontext.ContextSentRecord,
	// so it's re-encoded as a raw JSON value rather than a base64 string
	// when this document itself is marshaled to JSON for the API.
	ContextSent json.RawMessage `bson:"context_sent,omitempty" json:"context_sent,omitempty"`
	// ToolCall is set only on an assistant message whose completion
	// produced one — JSON-encoded provider.ToolCall.
	ToolCall json.RawMessage `bson:"tool_call,omitempty" json:"tool_call,omitempty"`

	// PageKey, System, History, and Tools are the generation input for a
	// pending assistant message — everything StreamMessage needs to run the
	// completion, stored rather than held in memory so it survives a
	// server restart between CreatePendingMessage and StreamMessage.
	// Cleared once the message completes; there is nothing left to resume.
	// Excluded from the JSON API entirely (json:"-") — this is internal
	// generation plumbing, not something a client needs to see.
	PageKey string          `bson:"page_key,omitempty" json:"-"`
	System  string          `bson:"system,omitempty" json:"-"`
	History json.RawMessage `bson:"history,omitempty" json:"-"` // []provider.Message
	Tools   json.RawMessage `bson:"tools,omitempty" json:"-"`   // []provider.ToolSpec

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// ChatbotRepo stores chat sessions' messages.
type ChatbotRepo struct {
	collection *mongo.Collection
}

// NewChatbotRepo opens the chatbot_messages collection in database.
func NewChatbotRepo(client *mongo.Client, database string) *ChatbotRepo {
	return &ChatbotRepo{collection: client.Database(database).Collection(chatbotMessageCollection)}
}

// EnsureIndexes creates the indexes message history and session listing
// depend on. Idempotent, safe to call on every startup.
func (r *ChatbotRepo) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "session_id", Value: 1}, {Key: "created_at", Value: 1}},
			Options: options.Index().SetName("session_created_at"),
		},
	}
	if _, err := r.collection.Indexes().CreateMany(ctx, indexes); err != nil {
		return fmt.Errorf("mongostore: creating %s indexes: %w", chatbotMessageCollection, err)
	}
	return nil
}

// Insert persists msg, filling in CreatedAt and ID.
func (r *ChatbotRepo) Insert(ctx context.Context, msg *ChatMessage) error {
	msg.CreatedAt = time.Now().UTC()
	result, err := r.collection.InsertOne(ctx, msg)
	if err != nil {
		return fmt.Errorf("mongostore: inserting chat message: %w", err)
	}
	msg.ID = result.InsertedID.(bson.ObjectID)
	return nil
}

// Get fetches one message by id.
func (r *ChatbotRepo) Get(ctx context.Context, id bson.ObjectID) (*ChatMessage, error) {
	var msg ChatMessage
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&msg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongostore: fetching chat message %s: %w", id.Hex(), err)
	}
	return &msg, nil
}

// MarkComplete finalizes a pending assistant message with its generated
// text and (if the model called one) tool call, and clears the generation
// input fields — there is nothing left to resume once a message is no
// longer pending.
func (r *ChatbotRepo) MarkComplete(ctx context.Context, id bson.ObjectID, status, text string, toolCall []byte) error {
	update := bson.M{
		"$set": bson.M{
			"status": status,
			"text":   text,
		},
		"$unset": bson.M{
			"page_key": "",
			"system":   "",
			"history":  "",
			"tools":    "",
		},
	}
	if len(toolCall) > 0 {
		update["$set"].(bson.M)["tool_call"] = toolCall
	}

	result, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return fmt.Errorf("mongostore: marking chat message %s complete: %w", id.Hex(), err)
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ListMessages returns every message in a session, oldest first.
func (r *ChatbotRepo) ListMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	cursor, err := r.collection.Find(ctx,
		bson.M{"session_id": sessionID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("mongostore: listing chat messages: %w", err)
	}

	messages := []ChatMessage{}
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("mongostore: decoding chat messages: %w", err)
	}
	return messages, nil
}

// SessionSummary is one row of the session list — the session's id and
// when its most recent message landed, for a "recent chats" view.
type SessionSummary struct {
	SessionID     string    `bson:"_id" json:"session_id"`
	LastMessageAt time.Time `bson:"last_message_at" json:"last_message_at"`
}

// ListSessions returns every session that has at least one message,
// most-recently-active first.
func (r *ChatbotRepo) ListSessions(ctx context.Context) ([]SessionSummary, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":             "$session_id",
			"last_message_at": bson.M{"$max": "$created_at"},
		}}},
		{{Key: "$sort", Value: bson.M{"last_message_at": -1}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongostore: listing chat sessions: %w", err)
	}

	sessions := []SessionSummary{}
	if err := cursor.All(ctx, &sessions); err != nil {
		return nil, fmt.Errorf("mongostore: decoding chat sessions: %w", err)
	}
	return sessions, nil
}
