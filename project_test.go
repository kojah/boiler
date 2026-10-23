package boiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/boiler"
)

// TestProjectManifest covers shared recipes, manifest-relative paths, glob
// overlap, deterministic source order and equivalence with standalone input.
func TestProjectManifest(t *testing.T) {
	standalone := module(t, "Name, inner = keep\nTags = clone\n")
	config, err := boiler.Load(standalone)
	if err != nil {
		t.Fatal(err)
	}
	original, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(standalone)
	if err != nil {
		t.Fatal(err)
	}
	_, body, _ := strings.Cut(string(source), "\n")
	root := filepath.Dir(standalone)
	// Create later-sorting input first; its filename need not equal a Go package.
	write(t, filepath.Join(root, "boiler", "z-other.boiler"), strings.ReplaceAll(body, "snapshotItem", "snapshotOther"))
	other := filepath.Join(root, "boiler", "z-other.boiler")
	changed, err := os.ReadFile(other)
	if err != nil {
		t.Fatal(err)
	}
	write(t, other, strings.ReplaceAll(string(changed), "boiler_items_gen.go", "boiler_other_gen.go"))
	write(t, filepath.Join(root, "boiler", "a-items.boiler"), body)
	path := filepath.Join(root, "boiler.yaml")
	write(t, path, `version: 1
sources: [boiler/*.boiler, boiler/a-items.boiler]
recipes:
  snapshot: recipes/snapshot
`)
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 || artifacts[0].Output != "p/boiler_items_gen.go" || artifacts[1].Output != "p/boiler_other_gen.go" {
		t.Fatalf("wrong deduplicated source order: %+v", artifacts)
	}
	if artifacts[0].Path != original[0].Path || string(artifacts[0].Data) != string(original[0].Data) {
		t.Fatal("project loading changed generated path or contents")
	}
	repeated, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	for i := range artifacts {
		if string(artifacts[i].Data) != string(repeated[i].Data) {
			t.Fatal("project generation is not deterministic")
		}
	}
}

func TestProjectSourceErrors(t *testing.T) {
	for _, test := range []struct{ name, manifest, source, want string }{
		{"unmatched", "version: 1\nsources: [missing/*.boiler]\n", "", "matched no files"},
		{"malformed-pattern", "version: 1\nsources: ['[']\n", "", "syntax error in pattern"},
		{"recursive-pattern", "version: 1\nsources: ['**/*.boiler']\n", "", "recursive ** globs are not supported"},
		{"escape", "version: 1\nsources: ['../*.boiler']\n", "", "must stay inside"},
		{"empty-sources", "version: 1\nsources: []\n", "", "nonempty sources"},
		{"unknown-setting", "version: 1\nsources: [source.boiler]\nunknown: true\n", "", "field unknown"},
		{"syntax-position", "version: 1\nsources: [source.boiler]\n", "invalid", "source.boiler:1:1"},
		{"duplicate-alias", "version: 1\nsources: [source.boiler]\nrecipes: {r: recipes/r}\n", "recipe r = \"other\"\n", "declared in both"},
		{"duplicate-setting", "version: 1\nsources: [source.boiler]\nreadonly: views\n", "readonly = \"other\"\n", "readonly is also declared"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "go.mod"), "module example.com/project\n")
			path := filepath.Join(root, "boiler.yaml")
			write(t, path, test.manifest)
			write(t, filepath.Join(root, "source.boiler"), test.source)
			if _, err := boiler.Load(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error %v, want %q", err, test.want)
			}
		})
	}
}
