package boiler

import (
	"fmt"
	"go/parser"
	"strings"
	"text/template"
)

func (e *expansion) funcs() template.FuncMap {
	return template.FuncMap{
		"fresh": e.fresh, "emit": e.emit, "child": e.child, "targetFor": e.targetFor,
		"import": e.u.useImport, "qualified": e.qualified, "structFields": e.structFields,
		"assert": func(ok bool, message string) (string, error) {
			if !ok {
				return "", fmt.Errorf("%s", message)
			}
			return "", nil
		},
		"ident": func(name string) (string, error) {
			if !identifier(name) {
				return "", fmt.Errorf("%q is not an identifier", name)
			}
			return name, nil
		},
		"arg": func(r *Rule, index int, fallback string) string {
			if index >= 0 && index < len(r.Args) {
				return r.Args[index]
			}
			return fallback
		},
		"comment": func(text string) string { return "// " + strings.ReplaceAll(strings.TrimSpace(text), "\n", "\n// ") },
		"dict":    dictionary, "put": putOnce, "take": take, "empty": empty,
	}
}

func (e *expansion) qualified(name string) (string, error) {
	expr, err := parser.ParseExpr(name)
	if err != nil {
		return "", err
	}
	for _, q := range qualifiers(expr) {
		if _, err := e.u.use(q, nil); err != nil {
			return "", err
		}
	}
	return name, nil
}

func (e *expansion) structFields(typ string) ([]*fieldData, error) {
	fields, src, err := e.u.structOf(typ)
	if err != nil {
		return nil, err
	}
	result := make([]*fieldData, 0, len(fields))
	for _, f := range fields {
		result = append(result, &fieldData{Name: f.name, Shape: e.u.shape(src, f, f.typ, 0)})
	}
	return result, nil
}

// Template dictionaries support checked one-to-one bindings. This is shared
// bookkeeping, not knowledge of projection recipes or their field rules.
func dictionary(args ...any) (map[string]any, error) {
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("dict requires key/value pairs")
	}
	out := map[string]any{}
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key must be a string")
		}
		if _, err := putOnce(out, key, args[i+1]); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func putOnce(values map[string]any, key string, value any) (string, error) {
	if _, ok := values[key]; ok {
		return "", fmt.Errorf("binding %s is set twice", key)
	}
	values[key] = value
	return "", nil
}
func take(values map[string]any, key string) (any, error) {
	value, ok := values[key]
	if !ok {
		return nil, fmt.Errorf("binding %s is missing or already used", key)
	}
	delete(values, key)
	return value, nil
}
func empty(values map[string]any) (string, error) {
	if len(values) > 0 {
		return "", fmt.Errorf("unused bindings: %s", strings.Join(sortedKeys(values), ", "))
	}
	return "", nil
}
