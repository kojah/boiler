package boiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kojah/boiler/internal/syntax"
)

// projectConfig owns discovery and shared settings, never generation targets.
// Relative paths share the manifest directory as their base.
type projectConfig struct {
	Version  int               `yaml:"version"`
	Sources  []string          `yaml:"sources"`
	Readonly *string           `yaml:"readonly"`
	Recipes  map[string]string `yaml:"recipes"`
}

// configInput separates filesystem discovery from the pure syntax frontend.
type configInput struct {
	files      []string
	base, path string
	project    *projectConfig
}

func discoverConfig(path string) (configInput, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return configInput{}, err
	}
	input := configInput{base: filepath.Dir(absolute), path: absolute}
	switch filepath.Ext(absolute) {
	case ".boiler":
		input.files = []string{absolute}
	case ".yaml", ".yml":
		input.project = &projectConfig{}
		if err := readYAML(absolute, input.project); err != nil {
			return input, err
		}
		if input.project.Version != 1 || len(input.project.Sources) == 0 {
			return input, fmt.Errorf("%s: project config requires version 1 and nonempty sources", absolute)
		}
		input.files, err = sourceFiles(input.base, input.project.Sources)
		if err != nil {
			return input, fmt.Errorf("%s: %w", absolute, err)
		}
	default:
		return input, fmt.Errorf("%s: expected a YAML project config or standalone .boiler file", path)
	}
	return input, nil
}

// sourceFiles expands Go filepath globs and deduplicates overlapping patterns.
// Global filename order, not pattern or filesystem order, controls compilation.
func sourceFiles(base string, patterns []string) ([]string, error) {
	seen := map[string]bool{}
	for _, pattern := range patterns {
		if !localPath(pattern) {
			return nil, fmt.Errorf("source pattern %q must stay inside the configuration directory", pattern)
		}
		if strings.Contains(pattern, "**") {
			return nil, fmt.Errorf("source pattern %q: recursive ** globs are not supported", pattern)
		}
		matches, err := filepath.Glob(filepath.Join(base, pattern))
		if err != nil {
			return nil, fmt.Errorf("source pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("source pattern %q matched no files", pattern)
		}
		for _, path := range matches {
			info, err := os.Stat(path)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() || filepath.Ext(path) != ".boiler" {
				return nil, fmt.Errorf("source %s must be a regular .boiler file", path)
			}
			seen[path] = true
		}
	}
	return sortedKeys(seen), nil
}

// parseConfigFiles combines complete positioned trees before lowering, so
// shared aliases are available regardless of which filename sorts first.
func parseConfigFiles(paths []string) (*syntax.File, error) {
	combined := &syntax.File{}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		file, err := syntax.Parse(path, string(source))
		if err != nil {
			return nil, err
		}
		combined.Recipes = append(combined.Recipes, file.Recipes...)
		combined.Settings = append(combined.Settings, file.Settings...)
		combined.Blocks = append(combined.Blocks, file.Blocks...)
	}
	return combined, nil
}

func (input configInput) shared(config *Config, tree *syntax.File) error {
	if input.project == nil {
		return nil
	}
	if input.project.Readonly != nil {
		if len(tree.Settings) > 0 {
			return fmt.Errorf("%s: readonly is also declared in %s", tree.Settings[0].Pos, input.path)
		}
		config.Readonly = *input.project.Readonly
	}
	for _, name := range sortedKeys(input.project.Recipes) {
		if _, exists := config.Recipes[name]; exists {
			return fmt.Errorf("recipe %s is declared in both %s and its sources", name, input.path)
		}
		config.Recipes[name] = input.project.Recipes[name]
	}
	return nil
}
