// Package genboundary holds no code of its own. This file enforces the rule
// docs/adr/0005 records: proto is the single definition for any model that
// crosses a language boundary, so no Go or TypeScript type may be hand-written
// to mirror a generated message or enum. The next hand-written mirror fails
// the build rather than accumulating.
package genboundary

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// This is a source-level check, not a build-graph walk like
// internal/agent/boundary_test.go: the rule is about how a type is
// *declared* — an alias to the generated message versus a hand-written
// struct — which the dependency graph cannot see. It follows the idiom of
// internal/server/services/appconfig_boundary_test.go (read the source, look
// for the forbidden shape), scaled to the whole repository.
//
// The signal is a name collision. internal/genproto/metarr/v1/*.pb.go and
// ui/src/gen/metarr/v1/*_pb.ts define every generated message and enum type.
// A hand-written type *definition* of the same name outside those generated
// trees — `type <Name> struct` or `type <Name> string` in Go, `export
// type|interface|enum <Name>` in TypeScript — is a mirror by definition: the
// generated type already *is* that model. A correct migration replaces it
// with an alias (`type <Name> = metarrv1.<Name>`), which this does not flag,
// because an alias is not a definition.
//
// The check keys on the name, so a hand-written mirror given a *different*
// name than its proto type slips through — the enforcement it buys is that
// the canonical model names, which every removed mirror reused, cannot be
// re-declared. Enums are covered as well as messages, since a closed
// vocabulary is exactly the kind of thing tempting to re-type by hand.
//
// A false positive is a hand-written type that shares a name with an
// unrelated generated type without mirroring it — a domain object compiled
// from stored config, say, or a storage envelope. Each one lives in allowed
// below with the reason it is not a mirror. Anything not on that list is a
// violation: migrate it to an alias, or, if it genuinely is not a mirror,
// add it to the list with an explanation.

// allowed maps "<repo-relative file>::<TypeName>" to why that hand-written
// type is not a mirror of the generated message it shares a name with.
var allowed = map[string]string{
	"internal/shared/config/config.go::Config":                    "infra wiring loaded from config.yaml (Mongo URI, Redis, log forward URL) — unrelated to metarr.v1.Config, the stored application config",
	"internal/shared/config/agent.go::AgentConfig":                "the agent's tiny local file (Redis connection + slug) — unrelated to metarr.v1.AgentConfig, the server-side agent record",
	"internal/shared/scanmodel/sidecar.go::SidecarTypeDefinition": "the compiled, typed form (closed category vocabulary + compiled regexps) derived from the stored metarr.v1.SidecarTypeDefinition, not a wire or storage shape",
	"internal/server/mongostore/workflow_repo.go::Workflow":       "the versioned storage envelope; the graph it holds rides as the generated metarr.v1.WorkflowGraph, converted in the workflow service",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../internal/genboundary/mirrors_test.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// generatedGoMessageNames collects the name of every type defined in the
// generated Go proto package — message structs and enum int32 types alike.
func generatedGoMessageNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	genDir := filepath.Join(root, "internal", "genproto")
	err := filepath.WalkDir(genDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Assign.IsValid() {
					continue // ts.Assign set => alias; a .pb.go defines, never aliases
				}
				// Every top-level definition in a .pb.go is generated: a
				// message (struct) or an enum (a named int32). Both are models
				// that cross the boundary, so both are guarded.
				names[ts.Name.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking generated proto: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("found no generated Go message types; the check would be vacuous")
	}
	return names
}

// TestNoHandWrittenGoTypeMirrorsAGeneratedMessage scans every non-generated,
// non-test Go file for a type definition (struct or enum) whose name collides
// with a generated message or enum.
func TestNoHandWrittenGoTypeMirrorsAGeneratedMessage(t *testing.T) {
	root := repoRoot(t)
	generated := generatedGoMessageNames(t, root)

	roots := []string{"internal", "cmd", "api"}
	for _, sub := range roots {
		start := filepath.Join(root, sub)
		if _, err := os.Stat(start); err != nil {
			continue
		}
		err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "genproto" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts.Assign.IsValid() {
						continue // `type X = ...` is an alias — exactly the migrated shape
					}
					name := ts.Name.Name
					if !generated[name] {
						continue
					}
					if reason, ok := allowed[rel+"::"+name]; ok {
						t.Logf("allowed: %s defines %s — %s", rel, name, reason)
						continue
					}
					pos := fset.Position(ts.Pos())
					t.Errorf("%s:%d defines hand-written type %q, which is also a generated metarr.v1 message or enum.\n"+
						"Proto is the single definition for a model that crosses a language boundary (docs/adr/0005): "+
						"replace this with `type %s = metarrv1.%s`, or, if it is genuinely not a mirror, add "+
						"%q to allowed in this file with the reason.",
						rel, pos.Line, name, name, name, rel+"::"+name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", sub, err)
		}
	}
}

var (
	tsGeneratedRE = regexp.MustCompile(`(?m)^export type ([A-Za-z0-9_]+) = Message<|^export enum ([A-Za-z0-9_]+) \{`)
	tsHandRE      = regexp.MustCompile(`(?m)^export (?:type|interface|enum) ([A-Za-z0-9_]+)\b`)
)

// TestNoHandWrittenTSTypeMirrorsAGeneratedMessage is the TypeScript half. The
// two files whose whole purpose was mirroring — ui/src/api/types.ts and
// ui/src/pages/workflows/catalogTypes.ts — are gone; this keeps them, and any
// replacement, from coming back.
func TestNoHandWrittenTSTypeMirrorsAGeneratedMessage(t *testing.T) {
	root := repoRoot(t)
	uiSrc := filepath.Join(root, "ui", "src")
	if _, err := os.Stat(uiSrc); err != nil {
		t.Skipf("no ui/src tree: %v", err)
	}

	generated := map[string]bool{}
	genDir := filepath.Join(uiSrc, "gen")
	err := filepath.WalkDir(genDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_pb.ts") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tsGeneratedRE.FindAllStringSubmatch(string(content), -1) {
			// Group 1 is a message name, group 2 an enum name; exactly one is
			// set per match.
			if m[1] != "" {
				generated[m[1]] = true
			}
			if m[2] != "" {
				generated[m[2]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking ui/src/gen: %v", err)
	}
	if len(generated) == 0 {
		t.Fatal("found no generated TS message types; the check would be vacuous")
	}

	err = filepath.WalkDir(uiSrc, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "gen" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		if strings.Contains(path, ".test.") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, m := range tsHandRE.FindAllStringSubmatch(string(content), -1) {
			if generated[m[1]] {
				t.Errorf("%s hand-declares %q, which is a generated metarr.v1 message type. "+
					"Import it from ui/src/gen instead — proto is the single definition (docs/adr/0005).",
					rel, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking ui/src: %v", err)
	}
}
