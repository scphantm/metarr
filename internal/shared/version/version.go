package version

import (
	"regexp"
	"strconv"
	"sync"
)

// Raw identifies the build. Set via -ldflags "-X Metarr/internal/shared/version.Raw=..."
// at build time; a dev build says so rather than claiming a number.
var Raw = "dev"

// Semver represents a parsed semantic version.
type Semver struct {
	Major int    // Major version number
	Minor int    // Minor version number
	Patch int    // Patch version number
	Pre   string // Prerelease suffix (e.g., "rc1", "beta", "" for release)
	Raw   string // Original unparsed string
	IsDev bool   // True if this is a dev build (unparseable or "dev")
}

var (
	semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-(.+))?$`)
	current  Semver
	once     sync.Once
)

// Parse attempts to parse s as a semantic version. If s is unparseable or
// "dev", it returns a Semver with IsDev set to true — a development build is
// a normal, expected state and should not be treated as an error.
func Parse(s string) Semver {
	if s == "dev" || s == "" {
		return Semver{Raw: s, IsDev: true}
	}

	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return Semver{Raw: s, IsDev: true}
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	pre := m[4]

	return Semver{
		Major: major,
		Minor: minor,
		Patch: patch,
		Pre:   pre,
		Raw:   s,
		IsDev: false,
	}
}

// Current returns the parsed build version, computed once on first call.
func Current() Semver {
	once.Do(func() {
		current = Parse(Raw)
	})
	return current
}

// String returns the original unparsed string.
func (s Semver) String() string {
	return s.Raw
}

// Compare returns -1 if s < other, 0 if equal, 1 if s > other.
// Dev builds sort last (greater than any release). Among releases,
// versions without a prerelease sort after those with one (1.0.0 > 1.0.0-rc1).
func (s Semver) Compare(other Semver) int {
	if s.IsDev && other.IsDev {
		return 0
	}
	if s.IsDev {
		return 1
	}
	if other.IsDev {
		return -1
	}

	if s.Major != other.Major {
		if s.Major < other.Major {
			return -1
		}
		return 1
	}
	if s.Minor != other.Minor {
		if s.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if s.Patch != other.Patch {
		if s.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Both have same major.minor.patch; compare prerelease:
	// no prerelease sorts after prerelease (e.g., 1.0.0 > 1.0.0-rc1)
	hasPreS := s.Pre != ""
	hasPreOther := other.Pre != ""
	if !hasPreS && hasPreOther {
		return 1
	}
	if hasPreS && !hasPreOther {
		return -1
	}
	if hasPreS && hasPreOther && s.Pre != other.Pre {
		if s.Pre < other.Pre {
			return -1
		}
		return 1
	}

	return 0
}

// AtLeast returns true if s >= the given major.minor.patch.
// Dev builds are never AtLeast any release version.
func (s Semver) AtLeast(major, minor, patch int) bool {
	if s.IsDev {
		return false
	}
	other := Semver{Major: major, Minor: minor, Patch: patch}
	return s.Compare(other) >= 0
}
