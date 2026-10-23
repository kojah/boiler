package boiler

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"text/template"
)

// recipe is a local, declarative template package. The engine has no registry
// of built-in recipe or rule names.
type recipe struct {
	Version   int                   `yaml:"version"`
	Entry     string                `yaml:"entry"`
	Templates []string              `yaml:"templates"`
	Rules     string                `yaml:"rules"`
	Local     bool                  `yaml:"local"`
	Options   map[string]optionSpec `yaml:"options"`
	rules     map[string]ruleSpec
}

type optionSpec struct {
	Kind     string `yaml:"kind"`
	Default  any    `yaml:"default"`
	Required bool   `yaml:"required"`
}

type ruleSpec struct {
	Template string   `yaml:"template"`
	Args     []int    `yaml:"args"`
	Accepts  []string `yaml:"accepts"`
	// Children selects the shape each nested rule receives: element or key.
	Children []string `yaml:"children"`
}

func (u *unit) loadRecipe(name string, funcs template.FuncMap) (*recipe, *template.Template, error) {
	files, err := u.recipeFiles(name)
	if err != nil {
		return nil, nil, err
	}
	var r recipe
	if err := readRecipeYAML(files, "recipe.yaml", &r); err != nil {
		return nil, nil, err
	}
	if r.Version != 1 || r.Entry == "" || !localPath(r.Rules) || len(r.Templates) == 0 {
		return nil, nil, fmt.Errorf("recipe %s requires version 1, entry, templates and a local rules file", name)
	}
	if err := readRecipeYAML(files, r.Rules, &r.rules); err != nil {
		return nil, nil, err
	}
	t, err := loadTemplates(name, files, r.Templates, funcs)
	if err != nil {
		return nil, nil, err
	}
	if t.Lookup(r.Entry) == nil {
		return nil, nil, fmt.Errorf("recipe %s has no entry template %s", name, r.Entry)
	}
	if err := r.validateRules(t); err != nil {
		return nil, nil, err
	}
	return &r, t, nil
}

func loadTemplates(name string, files fs.FS, patterns []string, funcs template.FuncMap) (*template.Template, error) {
	t := template.New(name).Option("missingkey=error").Funcs(funcs)
	seen := map[string]bool{}
	for _, pattern := range patterns {
		if !localPath(pattern) {
			return nil, fmt.Errorf("template pattern must be local: %s", pattern)
		}
		paths, err := fs.Glob(files, filepath.ToSlash(pattern))
		if err != nil || len(paths) == 0 {
			return nil, fmt.Errorf("template pattern %s matches no files or is invalid", pattern)
		}
		for _, path := range paths {
			base := filepath.Base(path)
			if seen[base] {
				return nil, fmt.Errorf("duplicate template filename %s", base)
			}
			seen[base] = true
			data, err := fs.ReadFile(files, path)
			if err != nil {
				return nil, err
			}
			if _, err := t.New(base).Parse(string(data)); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		}
	}

	return t, nil
}

func (r *recipe) validateRules(t *template.Template) error {
	for _, name := range sortedKeys(r.rules) {
		rule := r.rules[name]
		if rule.Template == "" {
			rule.Template = name
		}
		if len(rule.Args) == 0 {
			rule.Args = []int{0}
		}
		for _, n := range rule.Args {
			if n < 0 {
				return fmt.Errorf("rule %s: negative argument count", name)
			}
		}
		for _, kind := range rule.Accepts {
			if !slices.Contains([]string{"other", "pointer", "slice", "map"}, kind) {
				return fmt.Errorf("rule %s: unknown shape %s", name, kind)
			}
		}
		for _, child := range rule.Children {
			if child != "element" && child != "key" {
				return fmt.Errorf("rule %s: child must select element or key", name)
			}
		}
		if t.Lookup(rule.Template) == nil {
			return fmt.Errorf("rule %s: missing template %s", name, rule.Template)
		}
		r.rules[name] = rule
	}

	return nil
}
