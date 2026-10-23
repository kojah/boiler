package syntax_test

import (
	"strings"
	"testing"

	"github.com/kojah/boiler/internal/syntax"
)

// TestParse keeps the frontend independent of actual recipes and Go packages.
// The public AST preserves groups, nested rules, unbound options and locations.
func TestParse(t *testing.T) {
	source := `// Nothing named here needs to exist on disk.
recipe custom = "missing/recipe"
readonly = "internal/views"
output "p/boiler_example_gen.go" {
    custom Project(pkg.Record)
        receiver(p
            *publication)
        params(key map[string]Pair[int, string], other ID)
        doc("quoted, // text") declare(false) {
        First, Second = keep // grouped fields
        References = entries(
            call ref,
            each(call value)
        )
    }
}`
	file, err := syntax.Parse("example.boiler", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Blocks) != 1 {
		t.Fatalf("wrong output inventory: %+v", file)
	}
	output, ok := file.Blocks[0].(syntax.Output)
	if !ok || len(output.Targets) != 1 {
		t.Fatalf("wrong output block: %+v", file.Blocks[0])
	}
	target := output.Targets[0]
	if target.Recipe != "custom" || target.Type != "pkg.Record" || target.Pos.Line != 5 {
		t.Fatalf("wrong target: %+v", target)
	}
	if len(target.Fields) != 2 || strings.Join(target.Fields[0].Names, ",") != "First,Second" {
		t.Fatalf("lost field groups: %+v", target.Fields)
	}
	nested := target.Fields[1].Rule
	if nested.Name != "entries" || len(nested.Children) != 2 || nested.Children[1].Children[0].Args[0] != "value" {
		t.Fatalf("wrong nested rule: %+v", nested)
	}
	if len(target.Options) != 4 || target.Options[0].Arguments[0].Text != "p *publication" {
		t.Fatalf("wrong receiver arguments: %+v", target.Options)
	}
	params := target.Options[1].Arguments
	if len(params) != 2 || params[0].Text != "key map[string]Pair[int, string]" {
		t.Fatalf("split nested comma: %+v", params)
	}
	doc := target.Options[2].Arguments[0]
	if !doc.Quoted || doc.Text != "quoted, // text" {
		t.Fatalf("lost quoted text: %+v", doc)
	}
	if target.Options[3].Arguments[0].Quoted {
		t.Fatal("bare boolean marked quoted")
	}
}

func TestSyntaxErrors(t *testing.T) {
	for _, test := range []struct{ name, source, want string }{
		{"declaration", "recipes = x", "expected recipe"},
		{"unclosed-output", `output "p/boiler_x_gen.go" {`, "expected identifier"},
		{"unclosed-rule", "output \"p/boiler_x_gen.go\" { r F(T) { X = each(keep\n}", "expected , or )"},
		{"argument-delimiter", `output "p/boiler_x_gen.go" { r F(T) child([]T]) { X = keep } }`, "unmatched delimiter"},
		{"missing-separator", `output "p/boiler_x_gen.go" { r F(T) { X = keep Y = clone } }`, "expected newline"},
		{"unterminated-string", "recipe r = \"oops", "literal not terminated"},
		{"rule-depth", `output "p/boiler_x_gen.go" { r F(T) { X = ` + strings.Repeat("each(", 66) + "keep" + strings.Repeat(")", 66) + " } }", "nesting exceeds 64"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := syntax.Parse("bad.boiler", test.source)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "bad.boiler:") {
				t.Fatalf("Parse error %v; want positioned %q", err, test.want)
			}
		})
	}
}

func TestPackageBlocks(t *testing.T) {
	file, err := syntax.Parse("packages.boiler", `package "p" {
    output "boiler_one_gen.go" { r One(T) { X = keep } }
    output "boiler_two_gen.go" { r Two(T) { X = keep } }
}
output "q/boiler_three_gen.go" { r Three(T) { X = keep } }
package "p" { output "boiler_four_gen.go" { r Four(T) { X = keep } } }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Blocks) != 3 {
		t.Fatalf("got %d blocks", len(file.Blocks))
	}
	first, ok := file.Blocks[0].(syntax.Package)
	if !ok || first.Path != "p" || len(first.Outputs) != 2 || first.Pos.Line != 1 || first.Outputs[0].Path != "boiler_one_gen.go" {
		t.Fatalf("package scope was lost or paths resolved during parsing: %+v", file.Blocks[0])
	}
	middle, ok := file.Blocks[1].(syntax.Output)
	if !ok || middle.Path != "q/boiler_three_gen.go" {
		t.Fatalf("source order lost: %+v", file.Blocks)
	}
	for _, source := range []string{`package "p" {`, `package "p" { package "q" {} }`, `package "p" { recipe x = "r" }`} {
		if _, err := syntax.Parse("bad.boiler", source); err == nil || !strings.Contains(err.Error(), "expected output declaration in package") {
			t.Fatalf("Parse(%q) error %v", source, err)
		}
	}
}
