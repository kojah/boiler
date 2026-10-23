package boiler

import (
	"fmt"
	"go/parser"
	"go/token"
	"slices"
	"strings"
)

// binding is a parsed name/type pair used by receivers and parameters.
type binding struct{ Name, Type string }

func parseBinding(text string) (binding, error) {
	name, typ, ok := strings.Cut(strings.TrimSpace(text), " ")
	typ = strings.TrimSpace(typ)
	if !ok || !identifier(name) || name == "in" || name == "out" || typ == "" {
		return binding{}, fmt.Errorf("%q must be name Type, not named in or out", text)
	}
	if _, err := parser.ParseExpr(typ); err != nil {
		return binding{}, err
	}
	return binding{name, typ}, nil
}

func identifier(name string) bool { return name != "_" && token.IsIdentifier(name) }

func (u *unit) options(specs map[string]optionSpec, values map[string]any) (map[string]any, error) {
	result := map[string]any{}
	for _, name := range sortedKeys(values) {
		if _, ok := specs[name]; !ok {
			return nil, fmt.Errorf("unknown option %s", name)
		}
	}
	for _, name := range sortedKeys(specs) {
		spec := specs[name]
		value, ok := values[name]
		if !ok {
			value = spec.Default
		}
		if spec.Required && value == nil {
			return nil, fmt.Errorf("missing option %s", name)
		}
		if args, ok := value.(optionArguments); ok {
			bound, err := bindArguments(spec.Kind, args)
			if err != nil {
				return nil, fmt.Errorf("option %s: %w", name, err)
			}
			value = bound
		}
		parsed, err := u.option(spec.Kind, value)
		if err != nil {
			return nil, fmt.Errorf("option %s: %w", name, err)
		}
		result[name] = parsed
	}
	return result, nil
}

func (u *unit) option(kind string, value any) (any, error) {
	if kind == "bool" {
		if value == nil {
			return false, nil
		}
		v, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool")
		}
		return v, nil
	}
	if kind == "params" {
		return u.parameters(value)
	}
	if !slices.Contains([]string{"string", "identifier", "type", "receiver"}, kind) {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	text := ""
	if value != nil {
		var ok bool
		text, ok = value.(string)
		if !ok {
			return nil, fmt.Errorf("expected string")
		}
	}
	switch kind {
	case "identifier":
		if !identifier(text) {
			return nil, fmt.Errorf("%q is not an identifier", text)
		}
	case "type":
		if text == "" {
			return nil, fmt.Errorf("type is empty")
		}
		if err := u.useWritten(text); err != nil {
			return nil, err
		}
	case "receiver":
		if text == "" {
			return binding{}, nil
		}
		b, err := parseBinding(text)
		if err != nil {
			return nil, err
		}
		if !identifier(strings.TrimPrefix(b.Type, "*")) {
			return nil, fmt.Errorf("receiver requires a local type")
		}
		return b, nil
	}
	return text, nil
}

func sortedKeys[V any](values map[string]V) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (u *unit) parameters(value any) (any, error) {
	if value == nil {
		return []binding{}, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("expected list of name Type parameters")
	}
	result := []binding{}
	seen := map[string]bool{}
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("expected name Type")
		}
		b, err := parseBinding(text)
		if err != nil {
			return nil, err
		}
		if seen[b.Name] {
			return nil, fmt.Errorf("duplicate parameter %s", b.Name)
		}
		seen[b.Name] = true
		if err := u.useWritten(b.Type); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, nil
}
