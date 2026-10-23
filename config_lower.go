package boiler

import (
	"fmt"
	"path/filepath"

	"github.com/kojah/boiler/internal/syntax"
)

// Rule is a field action with word arguments or nested rules. Its meaning is
// supplied by a recipe; the configuration model does not encode recipe names.
type Rule struct {
	Name     string
	Args     []string
	Children []*Rule
}

type optionArgument struct {
	text   string
	quoted bool
}
type optionArguments []optionArgument

// lowerConfig is the boundary between source syntax and generation inputs.
// It expands field groups and checks duplicate bindings without loading recipes.
func lowerConfig(file *syntax.File) (*Config, error) {
	config := &Config{Recipes: map[string]string{}}
	settings := map[string]bool{}
	for _, setting := range file.Settings {
		if settings[setting.Name] {
			return nil, fmt.Errorf("%s: duplicate setting %s", setting.Pos, setting.Name)
		}
		settings[setting.Name] = true
		config.Readonly = setting.Value
	}
	for _, recipe := range file.Recipes {
		if _, exists := config.Recipes[recipe.Name]; exists {
			return nil, fmt.Errorf("%s: duplicate recipe %s", recipe.Pos, recipe.Name)
		}
		config.Recipes[recipe.Name] = recipe.Path
	}
	for _, block := range file.Blocks {
		outputs, err := lowerBlock(block)
		if err != nil {
			return nil, err
		}
		config.Outputs = append(config.Outputs, outputs...)
	}

	return config, nil
}

// lowerBlock preserves declaration order while resolving package-relative
// filenames. Package blocks represent one Go package, never a directory tree.
func lowerBlock(block syntax.Block) ([]Output, error) {
	switch node := block.(type) {
	case syntax.Output:
		output, err := lowerOutput("", node)
		if err != nil {
			return nil, err
		}
		return []Output{output}, nil
	case syntax.Package:
		if !localPath(node.Path) {
			return nil, fmt.Errorf("%s: package path must stay inside the configuration directory", node.Pos)
		}
		if len(node.Outputs) == 0 {
			return nil, fmt.Errorf("%s: package has no outputs", node.Pos)
		}
		outputs := make([]Output, 0, len(node.Outputs))
		for _, input := range node.Outputs {
			output, err := lowerOutput(node.Path, input)
			if err != nil {
				return nil, err
			}
			outputs = append(outputs, output)
		}
		return outputs, nil
	default:
		return nil, fmt.Errorf("unsupported configuration block %T", block)
	}
}

func lowerOutput(directory string, input syntax.Output) (Output, error) {
	result := Output{Package: filepath.Dir(input.Path), Out: input.Path}
	if directory != "" {
		if !localPath(input.Path) || filepath.Base(input.Path) != input.Path {
			return result, fmt.Errorf("%s: package output must be a filename, got %q", input.Pos, input.Path)
		}
		result.Package = filepath.Clean(directory)
		result.Out = filepath.Join(directory, input.Path)
	}
	for _, target := range input.Targets {
		lowered, err := lowerTarget(target)
		if err != nil {
			return result, err
		}
		result.Targets = append(result.Targets, lowered)
	}
	return result, nil
}

func lowerTarget(input syntax.Target) (Target, error) {
	target := Target{Recipe: input.Recipe, Name: input.Name, Type: input.Type, Options: map[string]any{}, Fields: map[string]*Rule{}}
	for _, option := range input.Options {
		if _, exists := target.Options[option.Name]; exists {
			return target, fmt.Errorf("%s: duplicate option %s", option.Pos, option.Name)
		}
		values := make(optionArguments, 0, len(option.Arguments))
		for _, arg := range option.Arguments {
			values = append(values, optionArgument{text: arg.Text, quoted: arg.Quoted})
		}
		target.Options[option.Name] = values
	}
	for _, group := range input.Fields {
		rule := lowerRule(group.Rule)
		for _, name := range group.Names {
			if _, exists := target.Fields[name]; exists {
				return target, fmt.Errorf("%s: field %s has more than one rule", group.Pos, name)
			}
			target.Fields[name] = rule
		}
	}
	return target, nil
}

func lowerRule(input *syntax.Rule) *Rule {
	result := &Rule{Name: input.Name, Args: input.Args}
	for _, child := range input.Children {
		result.Children = append(result.Children, lowerRule(child))
	}
	return result
}

func bindArguments(kind string, args optionArguments) (any, error) {
	if kind == "params" {
		result := make([]any, 0, len(args))
		for _, arg := range args {
			result = append(result, arg.text)
		}
		return result, nil
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("expected one argument, got %d", len(args))
	}
	arg := args[0]
	if kind == "bool" && !arg.quoted {
		switch arg.text {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	}
	return arg.text, nil
}
