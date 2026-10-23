package boiler_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/boiler"
)

func TestGroupedFieldRules(t *testing.T) {
	path := module(t, "Name, inner = keep\nTags = clone\n")
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	grouped, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expanded := strings.Replace(string(original), "Name, inner = keep", "Name = keep\ninner = keep", 1)
	write(t, path, expanded)
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	separate, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if string(grouped[0].Data) != string(separate[0].Data) {
		t.Fatal("grouping changed generated output")
	}
	write(t, path, strings.Replace(string(original), "Name, inner = keep", "Name, inner = keep\ninner = keep", 1))
	if _, err := boiler.Load(path); err == nil || !strings.Contains(err.Error(), "field inner has more than one rule") ||
		!strings.Contains(err.Error(), "generate.boiler:") {
		t.Fatalf("duplicate grouped field: %v", err)
	}
}

func TestConfigurationBindings(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"duplicate-recipe", "recipe x = \"a\"\nrecipe x = \"b\"\n", "duplicate recipe x"},
		{"duplicate-setting", "readonly = \"a\"\nreadonly = \"b\"\n", "duplicate setting readonly"},
		{"duplicate-option", `recipe x = "a"
output "p/boiler_x_gen.go" { x F(T) receiver(p *T) receiver(q *T) { A = keep } }`, "duplicate option receiver"},
		{"escaped-output", `recipe x = "a"
output "../boiler_x_gen.go" { x F(T) { A = keep } }`, "must stay inside"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "go.mod"), "module example.com/x\n")
			path := filepath.Join(root, "generate.boiler")
			write(t, path, test.source)
			if _, err := boiler.Load(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error %v, want %q", err, test.want)
			}
		})
	}
}

// TestPackageOutputs verifies the public path-resolution contract without
// changing generated code or making recipe paths package-relative.
func TestPackageOutputs(t *testing.T) {
	path := module(t, "Name, inner = keep\nTags = clone\n")
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	grouped := strings.Replace(string(text), `output "p/boiler_items_gen.go" {`, `package "p" { output "boiler_items_gen.go" {`, 1) + "}\n"
	write(t, path, grouped)
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("p/boiler_items_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != original[0].Path || string(artifacts[0].Data) != string(original[0].Data) {
		t.Fatal("package grouping changed generated path or contents")
	}
	for _, test := range []struct{ name, source, want string }{
		{"escape-package", strings.Replace(grouped, `package "p"`, `package "../p"`, 1), "package path must stay inside"},
		{"escape-output", strings.Replace(grouped, `output "boiler_items_gen.go"`, `output "../boiler_items_gen.go"`, 1), "package output must be a filename"},
		{"nested-directory", strings.Replace(grouped, `output "boiler_items_gen.go"`, `output "nested/boiler_items_gen.go"`, 1), "package output must be a filename"},
		{"absolute-output", strings.Replace(grouped, `output "boiler_items_gen.go"`, `output "/boiler_items_gen.go"`, 1), "package output must be a filename"},
		{"same-path", grouped + `output "p/boiler_items_gen.go" { snapshot Another(Item) { Name, Tags, inner = keep } }`, "duplicate output"},
		{"empty-package", `package "p" {}`, "package has no outputs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(t, path, test.source)
			if _, err := boiler.Load(path); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error %v, want %q", err, test.want)
			}
		})
	}
}
