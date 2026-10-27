package boiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/boiler"
)

// TestForeignTargets checks targets over another package's exported struct:
// that package's own type names print with the configured qualifier, so a
// pointer field resolves the matching snapshot target, and a key projects a
// foreign struct into a result owned by the output package.
func TestForeignTargets(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.24\n")
	write(t, filepath.Join(root, "q", "q.go"), `package q

type Outer struct {
	Inner *Inner
	Items []Item
	Index map[string]*Inner
	ID    int
}

type Inner struct{ N int }

type Item struct{ Name string }
`)
	write(t, filepath.Join(root, "p", "p.go"), `package p

import "example.com/m/q"

var _ q.Outer

type outerKey struct{ id int }
`)
	for _, recipe := range []string{"snapshot", "key"} {
		if err := os.CopyFS(filepath.Join(root, "recipes", recipe), os.DirFS(filepath.Join("recipes", recipe))); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "generate.boiler")
	write(t, path, `recipe snapshot = "recipes/snapshot"
recipe key = "recipes/key"
output "p/boiler_outer_gen.go" {
    snapshot snapshotOuter(q.Outer) {
        Inner = snapshot
        Items = clone
        Index = each(snapshot)
        ID = keep
    }
    snapshot snapshotInner(q.Inner) {
        N = keep
    }
    key keyOfOuter(q.Outer) result(outerKey) {
        ID = to id
        Inner, Items, Index = omit
    }
}
`)
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Publish(artifacts, false); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"v0 := snapshotInner(*out.Inner)",
		"v1 := snapshotInner(*e0)",
		"func keyOfOuter(in q.Outer) outerKey",
		"id: in.ID",
	} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("generated code lacks %q:\n%s", want, generated)
		}
	}
}
