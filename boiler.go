// Package boiler generates field-complete representation maintenance for Go
// structs: immutable snapshots, read-only views, comparable keys, and child
// visitors and transforms. A .boiler configuration names each struct and
// gives every one of its fields an explicit rule, so adding a field fails
// generation until each target says what to do with it. That coverage check
// is the point of the tool; the code it saves is secondary.
//
// Boiler reads Go source syntax only. It never loads or type-checks packages,
// so it can run before any other generator on a cold checkout; the ordinary
// Go build checks the generated code. Generated files are named
// boiler_*_gen.go, carry the standard generated-code header, and have no
// dependency on boiler. Semantic decisions belong to Hyrule rules and finite
// propagation to Daffle; boiler owns only the shape of the data.
//
// Main entry points are Load, Compile and Publish; cmd/boiler adapts them to
// the command line. README.md documents the configuration language.
package boiler

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config selects local recipes and groups generation targets into output files.
type Config struct {
	Readonly                 string
	Recipes                  map[string]string
	Outputs                  []Output
	root, moduleRoot, module string
}

// Output groups targets into a generated file; its path identifies the output.
type Output struct {
	Package string
	Out     string
	Targets []Target
}

// Target applies a recipe to a struct, with an explicit rule for every field.
type Target struct {
	Recipe  string
	Name    string
	Type    string
	Options map[string]any
	Fields  map[string]*Rule
}

var outputName = regexp.MustCompile(`^boiler_[a-z0-9_]+_gen\.go$`)

// Load reads a YAML project manifest and its .boiler source globs, or one
// standalone .boiler file. Paths are relative to the manifest/file directory.
// Source discovery is separate from parsing; recipe loading and Go inspection
// belong to Compile.
func Load(path string) (*Config, error) {
	input, err := discoverConfig(path)
	if err != nil {
		return nil, err
	}
	moduleRoot, module, err := findModule(input.base)
	if err != nil {
		return nil, err
	}
	tree, err := parseConfigFiles(input.files)
	if err != nil {
		return nil, err
	}
	c, err := lowerConfig(tree)
	if err != nil {
		return nil, err
	}
	if err := input.shared(c, tree); err != nil {
		return nil, err
	}
	c.root = input.base
	c.moduleRoot, c.module = moduleRoot, module
	return c, c.validate()
}

func (c *Config) validate() error {
	if c.Readonly != "" && !localPath(c.Readonly) {
		return fmt.Errorf("readonly must be a module-relative package path")
	}
	outputs := map[string]bool{}
	for _, t := range c.Outputs {
		if !localPath(t.Package) || !localPath(t.Out) {
			return fmt.Errorf("output %s: package and out must stay inside the module root", t.Out)
		}
		out := filepath.Clean(t.Out)
		if filepath.Dir(out) != filepath.Clean(t.Package) || !outputName.MatchString(filepath.Base(out)) {
			return fmt.Errorf("output %s: out must be boiler_*_gen.go inside the target package", t.Out)
		}
		if outputs[out] {
			return fmt.Errorf("output %s: duplicate output %s", t.Out, out)
		}
		outputs[out] = true
		if len(t.Targets) == 0 {
			return fmt.Errorf("output %s: no targets", t.Out)
		}
	}
	for _, name := range sortedKeys(c.Recipes) {
		recipePath := c.Recipes[name]
		if !identifier(name) || recipePath == "" {
			return fmt.Errorf("invalid recipe %q", name)
		}
	}
	for _, output := range c.Outputs {
		// A name may repeat across types: method recipes emit one name per
		// receiver type. Two functions sharing a name fail type checking.
		seen := map[[2]string]bool{}
		for _, target := range output.Targets {
			key := [2]string{target.Name, target.Type}
			if !identifier(target.Name) || seen[key] {
				return fmt.Errorf("output %s: invalid or duplicate target %q", output.Out, target.Name)
			}
			seen[key] = true
			if _, ok := c.Recipes[target.Recipe]; !ok {
				return fmt.Errorf("target %s: unknown recipe %q", target.Name, target.Recipe)
			}
		}
	}
	return nil
}

func localPath(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.IsLocal(path)
}

// findModule returns the nearest directory at or above dir holding go.mod,
// and that file's module path.
func findModule(dir string) (string, string, error) {
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
					return dir, strings.Trim(strings.TrimSpace(rest), `"`), nil
				}
			}
			return "", "", fmt.Errorf("%s has no module directive", filepath.Join(dir, "go.mod"))
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no go.mod at or above the config directory")
		}
		dir = parent
	}
}
