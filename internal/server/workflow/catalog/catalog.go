// Package catalog loads the hand-edited node type catalog the server owns.
//
// The catalog is the single source of truth shared by the editor's palette,
// server-side validation, and the engine. It used to live in the UI bundle,
// where the server could not see it — and a server that cannot see it can
// neither validate a graph nor execute one.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	metarrv1 "Metarr/internal/genproto/metarr/v1"
	"Metarr/internal/shared/workflow"
)

// Loader reads the catalog file and enforces the immutability rule across
// reloads.
type Loader struct {
	path string
	// hashes records the content hash seen for each type@version, so that a
	// silent edit to a published entry can be caught.
	hashes map[string]string
}

// NewLoader creates a loader for the catalog at path.
func NewLoader(path string) *Loader {
	return &Loader{path: path, hashes: make(map[string]string)}
}

// Path returns the file the loader reads.
func (l *Loader) Path() string { return l.path }

// Load parses and validates the catalog.
//
// catalog.json is a WorkflowCatalog message (node_types only), decoded with
// protojson so the on-disk file is the same shape the wire and the rest of
// the system read — there is no hand-written mirror of it anywhere. Every
// entry is validated, and a single bad one fails the whole load: a typo
// should surface at startup rather than when somebody happens to drag that
// node onto a canvas.
func (l *Loader) Load() (*workflow.Catalog, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("catalog: reading %s: %w", l.path, err)
	}

	var file metarrv1.WorkflowCatalog
	if err := protojson.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("catalog: parsing %s: %w", l.path, err)
	}

	loaded, err := workflow.NewCatalog(file.NodeTypes)
	if err != nil {
		return nil, fmt.Errorf("catalog: %s: %w", l.path, err)
	}

	if err := l.checkImmutability(file.NodeTypes); err != nil {
		return nil, err
	}
	return loaded, nil
}

// checkImmutability refuses a catalog in which a published entry has changed
// content.
//
// This matters more here than it usually would, because the catalog is a
// hand-edited file with no review process, and a run in flight has frozen a
// snapshot of the entries it uses. Silently changing what a given catalog id
// means would make two runs of the same workflow behave differently with
// nothing to show why. A behaviour change requires a new id.
func (l *Loader) checkImmutability(entries []*workflow.NodeType) error {
	marshal := proto.MarshalOptions{Deterministic: true}
	for _, entry := range entries {
		encoded, err := marshal.Marshal(entry)
		if err != nil {
			return fmt.Errorf("catalog: hashing %s (%s): %w", entry.Type, entry.Id, err)
		}
		sum := sha256.Sum256(encoded)
		hash := hex.EncodeToString(sum[:])

		previous, seen := l.hashes[entry.Id]
		if seen && previous != hash {
			return fmt.Errorf(
				"catalog: %s (%s) changed — a catalog entry must not change silently once "+
					"published, because runs in flight have frozen the previous definition; "+
					"give it a new id instead", entry.Type, entry.Id)
		}
		l.hashes[entry.Id] = hash
	}
	return nil
}
