package mongostore

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TaskEventRecord is the durable record of a task event having fired,
// persisted to the primary data store alongside the log line.
type TaskEventRecord struct {
	CorrelationID string    `bson:"correlation_id"`
	EventName     string    `bson:"event_name"`
	FiredAt       time.Time `bson:"fired_at"`
}

// TaskEventRepo persists TaskEventRecords to MongoDB.
type TaskEventRepo struct {
	collection *mongo.Collection
}

// NewTaskEventRepo opens the task events collection in database.
func NewTaskEventRepo(client *mongo.Client, database string) *TaskEventRepo {
	return &TaskEventRepo{
		collection: client.Database(database).Collection("task_events"),
	}
}

// Record inserts record as a new document.
func (r *TaskEventRepo) Record(ctx context.Context, record TaskEventRecord) error {
	_, err := r.collection.InsertOne(ctx, record)
	return err
}
