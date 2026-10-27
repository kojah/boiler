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

// TestMethodTargetsShareNames checks that a recipe emitting methods can use
// one target name for several receiver types, while a repeated name and
// type is still rejected.
func TestMethodTargetsShareNames(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.24\n")
	write(t, filepath.Join(root, "p", "p.go"), "package p\n\ntype A struct{ N int }\n\ntype B struct{ M, K int }\n")
	write(t, filepath.Join(root, "recipes", "sum", "recipe.yaml"),
		"version: 1\nentry: target\ntemplates: [\"*.go.tmpl\"]\nrules: rules.yaml\nlocal: true\noptions: {}\n")
	write(t, filepath.Join(root, "recipes", "sum", "rules.yaml"), "sum: {}\n")
	write(t, filepath.Join(root, "recipes", "sum", "target.go.tmpl"), `{{define "target" -}}
func (work *{{.Type}}) {{.Name}}(other {{.Type}}) {
{{range .Fields}}{{emit . .Name}}{{end -}}
}
{{end -}}
{{define "sum"}}work.{{.Dst}} += other.{{.Dst}}
{{end -}}
`)
	path := filepath.Join(root, "generate.boiler")
	targets := "sum Add(A) { N = sum }\nsum Add(B) { M, K = sum }\n"
	write(t, path, "recipe sum = \"recipes/sum\"\noutput \"p/boiler_sum_gen.go\" {\n"+targets+"}\n")
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func (work *A) Add(other A)", "func (work *B) Add(other B)", "work.K += other.K"} {
		if !strings.Contains(string(artifacts[0].Data), want) {
			t.Fatalf("generated code lacks %q:\n%s", want, artifacts[0].Data)
		}
	}
	write(t, path, "recipe sum = \"recipes/sum\"\noutput \"p/boiler_sum_gen.go\" {\n"+targets+"sum Add(A) { N = sum }\n}\n")
	if _, err := boiler.Load(path); err == nil || !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("repeated name and type accepted: %v", err)
	}
}
