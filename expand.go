package boiler

import (
	"bytes"
	"fmt"
	"go/ast"
	"slices"
	"strings"
	"text/template"
)

// typeShape exposes source syntax lazily: rendering Type registers imports,
// while Written can be compared without adding an unused import.
type typeShape struct {
	Kind, Written string
	u             *unit
	src           *source
	field         field
	expr          ast.Expr
	depth         int
}

func (s *typeShape) Type() (string, error) { return s.u.typeText(s.src, s.field, s.expr) }

// Resolve children only when requested. Eager expansion of recursive named
// maps (for example map[*M]*M) would grow exponentially before any rule ran.
func (s *typeShape) Element() *typeShape {
	_, elem, _ := s.src.shapeOf(s.expr)
	return s.u.shape(s.src, s.field, elem, s.depth+1)
}

func (s *typeShape) Key() *typeShape {
	_, _, key := s.src.shapeOf(s.expr)
	return s.u.shape(s.src, s.field, key, s.depth+1)
}

func (u *unit) shape(src *source, f field, expr ast.Expr, depth int) *typeShape {
	if expr == nil || depth > 64 {
		return nil
	}
	kind, _, _ := src.shapeOf(expr)
	return &typeShape{Kind: []string{"other", "pointer", "slice", "map"}[kind], Written: u.written(src, expr),
		u: u, src: src, field: f, expr: expr, depth: depth}
}

type targetData struct {
	Name, Type, Recipe, Readonly string
	Options                      map[string]any
	Fields                       []*fieldData
}

type fieldData struct {
	Name  string
	Shape *typeShape
	Rule  *Rule
}

type ruleContext struct {
	Target *targetData
	Field  *fieldData
	Rule   *Rule
	Shape  *typeShape
	Dst    string
	Depth  int
}

type expansion struct {
	names     map[string]bool
	u         *unit
	recipe    *recipe
	templates *template.Template
	target    *targetData
}

func (u *unit) expand(target Target) error {
	e := &expansion{u: u}
	r, t, err := u.loadRecipe(target.Recipe, e.funcs())
	if err != nil {
		return err
	}
	e.recipe = r
	e.templates = t
	if r.Local && strings.Contains(target.Type, ".") {
		return fmt.Errorf("recipe %s requires a local type", target.Recipe)
	}
	fields, src, err := u.structOf(target.Type)
	if err != nil {
		return err
	}
	parsed, err := rules(fields, target.Fields)
	if err != nil {
		return err
	}
	opts, err := u.options(r.Options, target.Options)
	if err != nil {
		return err
	}
	data := &targetData{Name: target.Name, Type: target.Type, Recipe: target.Recipe, Options: opts}
	if u.config.Readonly != "" {
		data.Readonly = u.config.module + "/" + u.config.Readonly
	}
	e.target = data
	for i, f := range fields {
		fd := &fieldData{Name: f.name, Shape: u.shape(src, f, f.typ, 0), Rule: parsed[i]}
		if err := e.validateRule(fd.Rule, fd.Shape, 0); err != nil {
			return fmt.Errorf("field %s: %w", f.name, err)
		}
		data.Fields = append(data.Fields, fd)
	}
	rendered, err := e.execute(r.Entry, data)
	if err != nil {
		return err
	}
	u.body.WriteString(rendered)
	u.body.WriteByte('\n')
	return nil
}

func (e *expansion) validateRule(rule *Rule, shape *typeShape, depth int) error {
	if depth > 64 {
		return fmt.Errorf("rule nesting exceeds 64")
	}
	spec, ok := e.recipe.rules[rule.Name]
	if !ok {
		return fmt.Errorf("unknown rule %q", rule.Name)
	}
	if !slices.Contains(spec.Args, len(rule.Args)) || len(spec.Children) != len(rule.Children) {
		return fmt.Errorf("rule %s needs argument count %v and %d nested rules", rule.Name, spec.Args, len(spec.Children))
	}
	if shape == nil {
		return fmt.Errorf("rule %s has no applicable shape", rule.Name)
	}
	if len(spec.Accepts) > 0 && !slices.Contains(spec.Accepts, shape.Kind) {
		return fmt.Errorf("%s needs a %s, not %s", rule.Name, strings.Join(spec.Accepts, " or "), shape.Written)
	}
	for i, child := range rule.Children {
		selected := shape.Element()
		if spec.Children[i] == "key" {
			selected = shape.Key()
		}
		if err := e.validateRule(child, selected, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (e *expansion) execute(name string, data any) (string, error) {
	var out bytes.Buffer
	if err := e.templates.ExecuteTemplate(&out, name, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (e *expansion) emit(field *fieldData, dst string) (string, error) {
	return e.renderRule(ruleContext{Target: e.target, Field: field, Rule: field.Rule, Shape: field.Shape, Dst: dst})
}

func (e *expansion) renderRule(ctx ruleContext) (string, error) {
	spec, ok := e.recipe.rules[ctx.Rule.Name]
	if !ok {
		return "", fmt.Errorf("unknown rule %s", ctx.Rule.Name)
	}
	out, err := e.execute(spec.Template, ctx)
	if err != nil {
		return "", fmt.Errorf("field %s: %w", ctx.Field.Name, err)
	}
	return out, nil
}

func (e *expansion) child(ctx ruleContext, index int, dst string) (string, error) {
	spec := e.recipe.rules[ctx.Rule.Name]
	if index < 0 || index >= len(spec.Children) {
		return "", fmt.Errorf("nested rule index %d is out of bounds", index)
	}
	shape := ctx.Shape.Element()
	if spec.Children[index] == "key" {
		shape = ctx.Shape.Key()
	}
	return e.renderRule(ruleContext{Target: ctx.Target, Field: ctx.Field, Rule: ctx.Rule.Children[index], Shape: shape, Dst: dst, Depth: ctx.Depth + 1})
}

// targetFor resolves one explicitly requested recipe/type pair in this output.
// It returns data, leaving call syntax and receiver compatibility to templates.
func (e *expansion) targetFor(recipeName, typ string) (*targetData, error) {
	var found *targetData
	for _, t := range e.u.targets {
		if t.Recipe != recipeName || t.Type != typ {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("multiple %s targets for %s", recipeName, typ)
		}
		r, _, err := e.u.loadRecipe(recipeName, e.funcs())
		if err != nil {
			return nil, err
		}
		options, err := e.u.options(r.Options, t.Options)
		if err != nil {
			return nil, err
		}
		found = &targetData{Name: t.Name, Type: t.Type, Recipe: t.Recipe, Options: options}
	}
	if found == nil {
		return nil, fmt.Errorf("no %s target for %s in this output", recipeName, typ)
	}
	return found, nil
}
