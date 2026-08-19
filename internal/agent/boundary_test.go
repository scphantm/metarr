// Package agent holds no code of its own. This file exists to enforce the one
// architectural rule the agent has to obey, and which nothing else can check:
// the agent runs on someone else's machine and must never reach the database.
package agent

import (
	"os/exec"
	"strings"
	"testing"
)

// forbidden lists import path fragments that must never appear anywhere in the
// agent's dependency graph.
//
// The Mongo rule from CLAUDE.md is about *connecting*: everything between the
// server and the agent goes over the event bus, so an agent holding a database
// handle has already broken the design. What that forbids is the driver's
// client packages — mongo, mongo/options, mongo/readpref — which are the only
// way to open a connection.
//
// It deliberately does not forbid mongo-driver/v2/bson. That package is a
// serialization codec in the same sense encoding/json is, and the shared scan
// records type their ids as bson.ObjectID so the server can store them
// unchanged. The agent never sets those ids — it has nothing to set them from
// — but the type travels with the model. Banning the codec outright would mean
// retyping every record id and migrating the _id of every document already in
// the database, which buys no safety: without the client packages there is
// nothing for the agent to connect with.
//
// internal/server is the corollary rule. Depending on server code is how a
// client import arrives by accident, since most server packages reach the
// database a hop or two down.
var forbidden = []struct {
	fragment string
	reason   string
}{
	{"go.mongodb.org/mongo-driver/v2/mongo", "the agent must never connect to MongoDB"},
	{"Metarr/internal/server/", "the agent must not depend on server-side packages"},
}

// TestAgentNeverDependsOnMongoOrServer walks the real build graph rather than
// the import blocks, so an indirect dependency several packages down is caught
// just as well as a direct one.
func TestAgentNeverDependsOnMongoOrServer(t *testing.T) {
	targets := []string{"./...", "../../cmd/metarr-agent"}

	for _, target := range targets {
		output, err := exec.Command("go", "list", "-deps", target).CombinedOutput()
		if err != nil {
			// A target that does not exist yet is not a boundary violation.
			text := string(output)
			if strings.Contains(text, "no Go files") ||
				strings.Contains(text, "cannot find") ||
				strings.Contains(text, "directory not found") ||
				strings.Contains(text, "matched no packages") {
				t.Logf("skipping %s: not built yet", target)
				continue
			}
			t.Fatalf("go list -deps %s: %v\n%s", target, err, output)
		}

		for _, dependency := range strings.Split(string(output), "\n") {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" {
				continue
			}
			for _, rule := range forbidden {
				if strings.Contains(dependency, rule.fragment) {
					t.Errorf(
						"%s depends on %s — %s",
						target, dependency, rule.reason,
					)
				}
			}
		}
	}
}
