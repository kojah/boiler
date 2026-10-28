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

// TestAliasTargets checks targets that name structs through aliases: a local
// alias, a chain across two packages into a package the output does not
// import, and fields typed through aliases finding the snapshot targets
// written for other spellings of the same struct. Signatures keep the alias.
func TestAliasTargets(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.24\n")
	write(t, filepath.Join(root, "d", "d.go"), `package d

type Work struct {
	Inner *Inner
	Steps []Step
	N     int
}

type Inner struct{ M int }

type Step struct{ Name string }

type Hidden struct{ secret int }
`)
	write(t, filepath.Join(root, "api", "api.go"), "package api\n\nimport \"example.com/m/d\"\n\ntype (\n\tWork   = d.Work\n\tInner  = d.Inner\n\tHidden = d.Hidden\n)\n")
	write(t, filepath.Join(root, "facts", "facts.go"), "package facts\n\nimport \"example.com/m/api\"\n\ntype (\n\tQueryWork = api.Work\n\tInner     = api.Inner\n\tHidden    = api.Hidden\n)\n")
	write(t, filepath.Join(root, "p", "p.go"), `package p

import "example.com/m/facts"

type state struct {
	work  facts.QueryWork
	local *Local
	count int
}

type local struct{ n int }

type Local = local
`)
	if err := os.CopyFS(filepath.Join(root, "recipes", "snapshot"), os.DirFS(filepath.Join("recipes", "snapshot"))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "generate.boiler")
	write(t, path, `recipe snapshot = "recipes/snapshot"
output "p/boiler_alias_gen.go" {
    snapshot snapshotState(state) {
        work, local = snapshot
        count = keep
    }
    snapshot snapshotWork(facts.QueryWork) {
        Inner = snapshot
        Steps = clone
        N = keep
    }
    snapshot snapshotInner(facts.Inner) {
        M = keep
    }
    snapshot snapshotLocal(Local) {
        n = keep
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
	generated := string(artifacts[0].Data)
	for _, want := range []string{
		"out.work = snapshotWork(out.work)",
		"v0 := snapshotLocal(*out.local)",
		"func snapshotWork(in facts.QueryWork) facts.QueryWork",
		"v0 := snapshotInner(*out.Inner)",
		"func snapshotInner(in facts.Inner) facts.Inner",
		"func snapshotLocal(in Local) Local",
	} {
		if !strings.Contains(generated, want) {
			t.Fatalf("generated code lacks %q:\n%s", want, generated)
		}
	}

	// A struct reached through an alias in another package keeps the
	// unexported-field rule and names the declaration.
	write(t, path, `recipe snapshot = "recipes/snapshot"
output "p/boiler_alias_gen.go" {
    snapshot snapshotHidden(facts.Hidden) {
        secret = keep
    }
}
`)
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Compile(""); err == nil || !strings.Contains(err.Error(), "alias of example.com/m/d.Hidden, which has unexported field secret") {
		t.Fatalf("unexported field through an alias accepted: %v", err)
	}
}
