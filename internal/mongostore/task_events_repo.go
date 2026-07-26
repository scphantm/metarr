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

type TaskEventRepo struct {
	collection *mongo.Collection
}

func NewTaskEventRepo(client *mongo.Client, database string) *TaskEventRepo {
	return &TaskEventRepo{
		collection: client.Database(database).Collection("task_events"),
	}
}

func (r *TaskEventRepo) Record(ctx context.Context, rec TaskEventRecord) error {
	_, err := r.collection.InsertOne(ctx, rec)
	return err
}
