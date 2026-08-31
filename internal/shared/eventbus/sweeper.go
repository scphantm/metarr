package eventbus

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultSweepInterval is how often the retention sweep runs. Well under the
// retention window, so a stream is never more than this far past its age
// floor, but infrequent enough that the repeated XTRIM cost is negligible.
const DefaultSweepInterval = time.Hour

// RetentionSweeper trims every known stream by age on an interval. Publish
// time caps a stream by entry count (RetentionPolicy.Maxlens); this bounds a
// low-volume stream that would otherwise keep months of history under its
// count cap.
type RetentionSweeper struct {
	client redis.UniversalClient
	policy RetentionPolicy
	logger *slog.Logger
	now    func() time.Time
}

// NewRetentionSweeper builds a sweeper for client using policy.
func NewRetentionSweeper(client redis.UniversalClient, policy RetentionPolicy, logger *slog.Logger) *RetentionSweeper {
	return &RetentionSweeper{
		client: client,
		policy: policy,
		logger: logger,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Run sweeps once immediately and then every interval until ctx is
// cancelled.
func (s *RetentionSweeper) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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

// SweepOnce trims every known stream so nothing older than the retention
// window survives. A per-stream trim failure is logged and the sweep moves
// on; only a context error stops it.
func (s *RetentionSweeper) SweepOnce(ctx context.Context) error {
	cutoff := s.now().Add(-time.Duration(s.policy.RetentionHours) * time.Hour)
	// A Redis Stream ID is "<unix-millis>-<sequence>"; MINID drops every
	// entry with an ID below this one.
	minID := strconv.FormatInt(cutoff.UnixMilli(), 10) + "-0"

	for _, stream := range sweepStreamNames(ctx, s.client) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		removed, err := s.client.XTrimMinIDApprox(ctx, stream, minID, 0).Result()
		if err != nil {
			s.logger.Warn("stream retention sweep: trim failed", "stream", stream, "error", err)
			continue
		}
		if removed > 0 {
			s.logger.Info("stream retention sweep trimmed old entries",
				"stream", stream, "removed", removed, "older_than", cutoff.Format(time.RFC3339))
		}
	}
	return nil
}
