package version

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  Semver
	}{
		{"dev", Semver{Raw: "dev", IsDev: true}},
		{"", Semver{Raw: "", IsDev: true}},
		{"1.2.3", Semver{Major: 1, Minor: 2, Patch: 3, Pre: "", Raw: "1.2.3"}},
		{"v1.2.3", Semver{Major: 1, Minor: 2, Patch: 3, Pre: "", Raw: "v1.2.3"}},
		{"1.2.3-rc1", Semver{Major: 1, Minor: 2, Patch: 3, Pre: "rc1", Raw: "1.2.3-rc1"}},
		{"v1.2.3-beta", Semver{Major: 1, Minor: 2, Patch: 3, Pre: "beta", Raw: "v1.2.3-beta"}},
		{"not-a-version", Semver{Raw: "not-a-version", IsDev: true}},
		{"1.2", Semver{Raw: "1.2", IsDev: true}},
	}

	for _, tt := range tests {
		got := Parse(tt.input)
		if got.Major != tt.want.Major || got.Minor != tt.want.Minor ||
			got.Patch != tt.want.Patch || got.Pre != tt.want.Pre ||
			got.Raw != tt.want.Raw || got.IsDev != tt.want.IsDev {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.input, got, tt.want)
		}
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // -1 if a < b, 0 if a == b, 1 if a > b
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		{"1.0.0-rc2", "1.0.0-rc1", 1},
		{"dev", "1.0.0", 1},
		{"1.0.0", "dev", -1},
		{"dev", "dev", 0},
	}

	for _, tt := range tests {
		a := Parse(tt.a)
		b := Parse(tt.b)
		got := a.Compare(b)
		if got != tt.want {
			t.Errorf("Parse(%q).Compare(Parse(%q)) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		patch   int
		want    bool
	}{
		{"1.2.3", 1, 2, 3, true},
		{"1.2.3", 1, 2, 2, true},
		{"1.2.3", 1, 2, 4, false},
		{"1.2.3", 1, 1, 3, true},
		{"1.2.3", 1, 3, 3, false},
		{"1.2.3", 0, 9, 9, true},
		{"1.2.3", 2, 0, 0, false},
		{"1.2.3-rc1", 1, 2, 2, true},
		{"1.2.3-rc1", 1, 2, 3, false},
		{"dev", 1, 0, 0, false},
	}

	for _, tt := range tests {
		v := Parse(tt.version)
		got := v.AtLeast(tt.major, tt.minor, tt.patch)
		if got != tt.want {
			t.Errorf("Parse(%q).AtLeast(%d, %d, %d) = %v, want %v",
				tt.version, tt.major, tt.minor, tt.patch, got, tt.want)
		}
	}
}
