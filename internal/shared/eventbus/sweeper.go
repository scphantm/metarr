package eventbus

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultSweepInterval is how often the retention sweep runs. Well under the
// retention window, so a stream is never more than this far past its age
// floor, but infrequent enough that the repeated XTRIM cost is negligible.
// It travels with the rest of the bus tuning as BusPolicy.SweepInterval.
const DefaultSweepInterval = time.Hour

// RetentionSweeper trims every discovered stream by age on an interval.
// Publish time caps a stream by entry count (RetentionPolicy.MaxLen); this
// bounds a low-volume stream that would otherwise keep months of history
// under its count cap.
type RetentionSweeper struct {
	client   redis.UniversalClient
	policy   RetentionPolicy
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time
}

// NewRetentionSweeper builds a sweeper for client that trims by
// policy.RetentionHours every interval. The interval is BusPolicy's
// SweepInterval slice — the sweeper takes only the tuning it uses.
func NewRetentionSweeper(client redis.UniversalClient, policy RetentionPolicy, interval time.Duration, logger *slog.Logger) *RetentionSweeper {
	return &RetentionSweeper{
		client:   client,
		policy:   policy,
		interval: interval,
		logger:   logger,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Run sweeps once immediately and then every SweepInterval until ctx is
// cancelled.
func (s *RetentionSweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		if err := s.SweepOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("stream retention sweep failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SweepOnce trims every stream DiscoverStreamTopics returns so nothing older
// than the retention window survives. A partial discovery failure (a failed
// per-agent SCAN) is logged and the streams it did find are still trimmed; a
// per-stream trim failure is logged and the sweep moves on; only a context
// error stops it.
func (s *RetentionSweeper) SweepOnce(ctx context.Context) error {
	cutoff := s.now().Add(-time.Duration(s.policy.RetentionHours) * time.Hour)
	// MINID drops every entry with an ID below this one.
	minID := StreamIDForTime(cutoff)

	topics, err := DiscoverStreamTopics(ctx, s.client)
	if err != nil {
		s.logger.Warn("stream retention sweep: stream discovery partially failed", "error", err)
	}

	for _, topic := range topics {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		removed, err := s.client.XTrimMinIDApprox(ctx, topic.Name, minID, 0).Result()
		if err != nil {
			s.logger.Warn("stream retention sweep: trim failed", "stream", topic.Name, "error", err)
			continue
		}
		if removed > 0 {
			s.logger.Info("stream retention sweep trimmed old entries",
				"stream", topic.Name, "removed", removed, "older_than", cutoff.Format(time.RFC3339))
		}
	}
	return nil
}
