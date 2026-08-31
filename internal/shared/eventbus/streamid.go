package eventbus

import (
	"strconv"
	"strings"
	"time"
)

// A Redis Stream entry ID is "<unix-millis>-<sequence>". These two helpers
// are the one place that shape is encoded and decoded — the retention sweep,
// the stats collector, and their tests all go through here rather than each
// re-deriving the format.

// StreamIDForTime returns the "<unix-millis>-0" ID for t. It is the low
// bound of an XTRIM MINID that should drop every entry published before t.
func StreamIDForTime(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10) + "-0"
}

// TimeFromStreamID recovers the publish time from a stream entry ID. The
// sequence part is ignored; ok is false for a malformed id.
func TimeFromStreamID(id string) (when time.Time, ok bool) {
	millisText, _, _ := strings.Cut(id, "-")
	millis, err := strconv.ParseInt(millisText, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.UnixMilli(millis).UTC(), true
}
