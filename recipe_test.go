package boiler_test

import (
	"bytes"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/boiler"
)

// TestLocalRecipe checks the extension contract through Load and Compile: a
// recipe unknown to the compiler produces valid Go and keeps field coverage.
func TestLocalRecipe(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/custom\n\ngo 1.24\n")
	const input = "package p\ntype Item struct { ID int; Secret string; Links Links }\ntype Links map[*Links]*Links\ntype strings struct{}\n"
	write(t, filepath.Join(root, "p", "p.go"), input)
	configPath := filepath.Join(root, "generate.boiler")
	write(t, configPath, `recipe names = "recipes/names"
output "p/boiler_names_gen.go" {
    names FieldNames(Item) {
        ID = label identity
        Secret, Links = omit
    }
}
`)
	recipePath := filepath.Join(root, "recipes", "names")
	write(t, filepath.Join(recipePath, "recipe.yaml"), `version: 1
entry: target
templates: ["*.go.tmpl"]
rules: rules.yaml
`)
	write(t, filepath.Join(recipePath, "rules.yaml"), "label: {args: [1]}\nomit: {}\n")
	const body = `{{define "target"}}
func {{.Name}}() string {
 return {{import "strings"}}.Join([]string{
 {{range .Fields}}{{emit . ""}}{{end}}
 }, ",")
}
{{end}}
{{define "label"}}{{printf "%q" (index .Rule.Args 0)}},{{end}}
{{define "omit"}}{{end}}
`
	templatePath := filepath.Join(recipePath, "target.go.tmpl")
	write(t, templatePath, body)
	config, err := boiler.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || !bytes.Equal(artifacts[0].Data, repeated[0].Data) {
		t.Fatal("generation is not deterministic")
	}
	fset := token.NewFileSet()
	original, err := parser.ParseFile(fset, "p.go", input, 0)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := parser.ParseFile(fset, "boiler_names_gen.go", artifacts[0].Data, 0)
	if err != nil {
		t.Fatal(err)
	}
	checker := types.Config{Importer: importer.Default()}
	if _, err := checker.Check("example.com/custom/p", fset, []*ast.File{original, generated}, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(artifacts[0].Data, []byte(`"identity"`)) || bytes.Contains(artifacts[0].Data, []byte(`"Secret"`)) {
		t.Fatalf("wrong field names: %s", artifacts[0].Data)
	}
	for _, test := range []struct{ name, template, want string }{
		{"missing-value", strings.Replace(body, "{{.Name}}", "{{.Options.typo}}", 1), "typo"},
		{"invalid-go", strings.Replace(body, "() string", "() ???", 1), "generated Go does not parse"},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(t, templatePath, test.template)
			if _, err := config.Compile(""); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile error %v, want %s", err, test.want)
			}
		})
	}
}

// TestTargetForFields checks that a resolved target exposes its fields and
// rules, so a composing template can account for a nested target's coverage.
func TestTargetForFields(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/nested\n\ngo 1.24\n")
	write(t, filepath.Join(root, "p", "p.go"), "package p\ntype Outer struct { In Inner; Tag string }\ntype Inner struct { A, B int }\n")
	configPath := filepath.Join(root, "generate.boiler")
	write(t, configPath, `recipe names = "recipes/names"
output "p/boiler_names_gen.go" {
    names OuterNames(Outer) {
        In = nested
        Tag = label tag
    }
    names InnerNames(Inner) {
        A = label first
        B = omit
    }
}
`)
	recipePath := filepath.Join(root, "recipes", "names")
	write(t, filepath.Join(recipePath, "recipe.yaml"), "version: 1\nentry: target\ntemplates: [\"*.go.tmpl\"]\nrules: rules.yaml\n")
	write(t, filepath.Join(recipePath, "rules.yaml"), "label: {args: [1]}\nnested: {}\nomit: {}\n")
	write(t, filepath.Join(recipePath, "target.go.tmpl"), `{{define "target"}}
func {{.Name}}() []string {
 return []string{ {{template "labels" .}} }
}
{{end}}
{{define "labels"}}{{$t := .}}{{range .Fields}}{{if eq .Rule.Name "label"}}{{printf "%q" (index .Rule.Args 0)}},{{else if eq .Rule.Name "nested"}}{{template "labels" (targetFor $t.Recipe .Shape.Written)}}{{end}}{{end}}{{end}}
{{define "label"}}{{end}}
{{define "nested"}}{{end}}
{{define "omit"}}{{end}}
`)
	config, err := boiler.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || !bytes.Contains(artifacts[0].Data, []byte(`[]string{"first", "tag"}`)) {
		t.Fatalf("nested target fields not exposed: %s", artifacts[0].Data)
	}
}

// TestBundledRecipe checks that portable recipes preserve local recipe output
// and require no recipe files in the consumer module.
func TestBundledRecipe(t *testing.T) {
	path := module(t, "Name, inner = keep\nTags = clone\n")
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	local, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, strings.Replace(string(input), "recipes/snapshot", "boiler:snapshot", 1))
	if err := os.RemoveAll(filepath.Join(filepath.Dir(path), "recipes")); err != nil {
		t.Fatal(err)
	}
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	bundled, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundled) != len(local) || !bytes.Equal(bundled[0].Data, local[0].Data) {
		t.Fatal("bundled recipe changed generated output")
	}
	write(t, path, strings.Replace(string(input), "recipes/snapshot", "boiler:missing", 1))
	config, err = boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Compile(""); err == nil {
		t.Fatal("missing bundled recipe accepted")
	}
}

// TestEqualRecipe checks the bundled equal recipe compares each field by its
// rule and leaves an omitted field uncompared.
func TestEqualRecipe(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.24\n")
	write(t, filepath.Join(root, "p", "p.go"), source)
	path := filepath.Join(root, "generate.boiler")
	write(t, path, "recipe equal = \"boiler:equal\"\noutput \"p/boiler_equal_gen.go\" {\n"+
		"equal equalItem(Item) {\nName = eq\nTags = each(eq)\ninner = omit\n}\n}\n")
	config, err := boiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	outputs, err := config.Compile("")
	if err != nil {
		t.Fatal(err)
	}
	generated := string(outputs[0].Data)
	for _, want := range []string{"left.Name != right.Name", "len(left.Tags) != len(right.Tags)", "left.Tags[i0] != right.Tags[i0]"} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated equality lacks %q:\n%s", want, generated)
		}
	}
	if strings.Contains(generated, "inner") {
		t.Errorf("omitted field compared:\n%s", generated)
	}
}
