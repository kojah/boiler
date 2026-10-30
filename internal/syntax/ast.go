// Package syntax parses .boiler source into a position-bearing syntax tree.
// It performs no file access, recipe loading, Go inspection or code generation.
// Parse preserves field groups and unbound options for later semantic validation.
package syntax

// Position identifies a byte offset and one-based line and column in the input.
type Position struct {
	Filename             string
	Offset, Line, Column int
}

// File is a parsed configuration, before names and recipe options are resolved.
type File struct {
	Recipes  []Recipe
	Settings []Setting
	Blocks   []Block
}

// Recipe binds a local alias to a directory path.
type Recipe struct {
	Name, Path string
	Pos        Position
}

// Setting is a file-wide string setting, currently readonly.
type Setting struct {
	Name, Value string
	Pos         Position
}

// Block is a source-ordered output or package declaration. Only syntax nodes
// implement it; path resolution belongs to configuration lowering.
type Block interface{ block() }

// Package groups output filenames under one written Go package directory.
type Package struct {
	Path    string
	Outputs []Output
	Pos     Position
}

func (Package) block() {}
func (Output) block()  {}

// Output groups generation targets under an output path.
type Output struct {
	Build   string
	Path    string
	Targets []Target
	Pos     Position
}

// Target applies a named recipe to a written source type.
type Target struct {
	Recipe, Name, Type string
	Options            []Option
	Fields             []FieldGroup
	Pos                Position
}

// Option contains arguments whose interpretation belongs to its recipe schema.
type Option struct {
	Name      string
	Arguments []Argument
	Pos       Position
}

// Argument preserves whether a value was explicitly quoted. Text is unquoted
// for string literals and otherwise contains the written expression.
type Argument struct {
	Text   string
	Quoted bool
	Pos    Position
}

// FieldGroup assigns one rule to one or more explicitly named fields.
type FieldGroup struct {
	Names []string
	Rule  *Rule
	Pos   Position
}

// Rule is a rule name with word arguments or recursively nested rules.
type Rule struct {
	Name     string
	Args     []string
	Children []*Rule
	Pos      Position
}
