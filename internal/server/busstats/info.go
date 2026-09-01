package busstats

import (
	"strconv"
	"strings"
)

// parseInfo turns the output of Redis's INFO command into a flat map.
//
// The format is line-oriented, CRLF-terminated, with "# Section" headers and
// blank lines between sections, both of which are skipped. Only the first
// colon separates key from value — several values contain colons themselves
// (db0:keys=1,expires=0), so splitting on every colon would corrupt them.
func parseInfo(raw string) map[string]string {
	fields := make(map[string]string)

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields[key] = value
	}

	return fields
}

// infoInt reads key from fields as an int64, returning zero when it is absent
// or not a number. A statistics endpoint should report a missing counter as
// zero rather than fail the whole snapshot over it.
func infoInt(fields map[string]string, key string) int64 {
	parsed, err := strconv.ParseInt(fields[key], 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
