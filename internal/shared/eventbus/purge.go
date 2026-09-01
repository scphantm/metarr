package eventbus

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// pelDrainBatch is how many pending IDs are read and acknowledged per round
// when clearing a group's pending list after the trim.
const pelDrainBatch = 500

// StreamPurge is the outcome of purging one durable stream: how many entries
// the approximate trim dropped, and the consumer groups it fast-forwarded to
// the stream's tail.
type StreamPurge struct {
	Stream           string
	Dropped          int64
	GroupsFastForwarded []string
}

// PurgeStream clears a jammed durable stream. It approximately trims every
// entry currently on the stream (XTRIM MINID at ~now), then for each of the
// stream's consumer groups it fast-forwards the group past the trim (XGROUP
// SETID to $) and drops the pending entries the trim left dangling, so a
// consumer resumes with nothing pending and no redelivery. The stream key and
// every group stay in place — see docs/adr/0007.
//
// It is deliberately a plain function over a Redis client, not a method on
// the sampler: the mechanics test on their own.
//
// A stream with no consumer groups — a reserved name nothing consumes yet, or
// one Redis has never created — is trimmed only and is not an error, so a
// purge-all batch does not break on it. Only a failed Redis call is an error.
func PurgeStream(ctx context.Context, client redis.UniversalClient, stream string) (StreamPurge, error) {
	result := StreamPurge{Stream: stream}

	// Every entry already on the stream has an ID below now, so a MINID trim
	// at now drops all of them. Approximate (~) because an exact trim scans
	// the stream and the small overshoot is irrelevant when the intent is
	// "drop everything up to now".
	minID := StreamIDForTime(time.Now().UTC())
	dropped, err := client.XTrimMinIDApprox(ctx, stream, minID, 0).Result()
	if err != nil {
		return result, fmt.Errorf("eventbus: trim stream %q: %w", stream, err)
	}
	result.Dropped = dropped

	groups, err := client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		if isNoSuchKey(err) {
			// Never created: the trim was a no-op and there is nothing to
			// fast-forward. Not an error.
			return result, nil
		}
		return result, fmt.Errorf("eventbus: read consumer groups on stream %q: %w", stream, err)
	}

	for _, group := range groups {
		if err := client.XGroupSetID(ctx, stream, group.Name, "$").Err(); err != nil {
			return result, fmt.Errorf("eventbus: fast-forward group %q on stream %q: %w", group.Name, stream, err)
		}
		if err := drainPending(ctx, client, stream, group.Name); err != nil {
			return result, err
		}
		result.GroupsFastForwarded = append(result.GroupsFastForwarded, group.Name)
	}
	return result, nil
}

// drainPending acknowledges every entry left in a group's pending list. After
// the trim those entries no longer exist on the stream, but Redis keeps their
// IDs in the PEL until they are acknowledged or claimed; XGROUP SETID does not
// clear them. Acknowledging them is what makes a purged group read as having
// nothing pending.
func drainPending(ctx context.Context, client redis.UniversalClient, stream, group string) error {
	for {
		pending, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: stream,
			Group:  group,
			Start:  "-",
			End:    "+",
			Count:  pelDrainBatch,
		}).Result()
		if err != nil {
			return fmt.Errorf("eventbus: list pending on group %q of stream %q: %w", group, stream, err)
		}
		if len(pending) == 0 {
			return nil
		}

		ids := make([]string, len(pending))
		for i, entry := range pending {
			ids[i] = entry.ID
		}
		if err := client.XAck(ctx, stream, group, ids...).Err(); err != nil {
			return fmt.Errorf("eventbus: acknowledge pending on group %q of stream %q: %w", group, stream, err)
		}
		if len(pending) < pelDrainBatch {
			return nil
		}
	}
}

// PurgeAllStreams purges every durable stream DiscoverStreamTopics resolves —
// the same discovered set the stats sampler reads — and returns one result
// per stream in discovery order. A partial discovery failure (a failed
// per-agent SCAN) still purges the streams it did find, matching how the
// retention sweep proceeds on a partial discovery. The first per-stream
// failure stops the batch and is returned with the results gathered so far;
// every entry in that slice is a completed purge.
func PurgeAllStreams(ctx context.Context, client redis.UniversalClient) ([]StreamPurge, error) {
	topics, _ := DiscoverStreamTopics(ctx, client)

	results := make([]StreamPurge, 0, len(topics))
	for _, topic := range topics {
		result, err := PurgeStream(ctx, client, topic.Name)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

// isNoSuchKey reports whether err is Redis's "no such key" error, which
// XINFO GROUPS returns for a stream that has not been created yet.
func isNoSuchKey(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such key")
}
