package boiler

import (
	"fmt"
	"go/ast"
)

// reservedNames prevents generated imports and fresh locals from shadowing
// identifiers already present in the source or configured target names.
func (u *unit) reservedNames() map[string]bool {
	names := map[string]bool{}
	for _, file := range u.pkg.files {
		ast.Inspect(file, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok {
				names[id.Name] = true
			}
			return true
		})
	}
	for _, target := range u.targets {
		names[target.Name] = true
	}
	for name := range u.allocated {
		names[name] = true
	}
	for name := range u.imports {
		names[name] = true
	}
	return names
}

func (e *expansion) fresh(prefix string) (string, error) {
	if !identifier(prefix) {
		return "", fmt.Errorf("fresh prefix %q is not an identifier", prefix)
	}
	if e.names == nil {
		e.names = e.u.reservedNames()
	}
	name := prefix
	for i := 2; e.names[name] || e.u.imports[name] != ""; i++ {
		name = fmt.Sprintf("%s%d", prefix, i)
	}
	e.names[name] = true
	e.u.allocated[name] = true
	return name, nil
}
