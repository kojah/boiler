package boiler

import (
	"fmt"
	"go/ast"
	"go/parser"
)

// Type resolution through aliases. Boiler reads syntax, not a type-checked
// build, so an alias `type A = B` is followed declaration by declaration:
// local names in their package, qualified names through the declaring file's
// imports. Packages outside the module end the chain where it stands.

// maxAliasHops bounds alias chains; Go rejects cyclic aliases, so the bound
// only guards malformed sources.
const maxAliasHops = 64

// unalias follows alias declarations from expr, written in file of src, and
// returns the package, file and expression the chain ends at; a named type
// ends it as a name in its declaring package. Generic aliases and unreadable
// packages end the chain where it stands.
func (l *loader) unalias(src *source, file *ast.File, expr ast.Expr) (*source, *ast.File, ast.Expr) {
	for range maxAliasHops {
		next := src
		var name string
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
			continue
		case *ast.Ident:
			name = e.Name
		case *ast.SelectorExpr:
			qualifier, ok := e.X.(*ast.Ident)
			if !ok || file == nil {
				return src, file, expr
			}
			importPath := importsOf(file)[qualifier.Name]
			if importPath == "" {
				return src, file, expr
			}
			loaded, err := l.loadImport(importPath)
			if err != nil {
				return src, file, expr
			}
			next, name = loaded, e.Sel.Name
		default:
			return src, file, expr
		}
		spec := next.types[name]
		if spec == nil || spec.TypeParams != nil {
			return src, file, expr
		}
		if !spec.Assign.IsValid() {
			// A qualified chain ends at its declaring package.
			return next, next.owner[spec], &ast.Ident{Name: name}
		}
		src, file, expr = next, next.owner[spec], spec.Type
	}
	return src, file, expr
}

// shapeOf classifies a field type written in file of src and returns its
// element and key types with the package and file they are written in.
// Aliases are transparent; a named type declared in src itself is followed
// one level to its declared container.
func (l *loader) shapeOf(src *source, file *ast.File, expr ast.Expr) (shape, ast.Expr, ast.Expr, *source, *ast.File) {
	rsrc, rfile, resolved := l.unalias(src, file, expr)
	if ident, ok := resolved.(*ast.Ident); ok && rsrc == src {
		if spec := src.types[ident.Name]; spec != nil && spec.TypeParams == nil {
			resolved, rfile = spec.Type, src.owner[spec]
		}
	}
	switch e := resolved.(type) {
	case *ast.StarExpr:
		return pointerShape, e.X, nil, rsrc, rfile
	case *ast.ArrayType:
		if e.Len == nil {
			return sliceShape, e.Elt, nil, rsrc, rfile
		}
	case *ast.MapType:
		return mapShape, e.Value, e.Key, rsrc, rfile
	}
	return otherShape, nil, nil, rsrc, rfile
}

// namedStruct is a struct declaration reached from a written type name.
type namedStruct struct {
	src     *source
	name    string
	aliased bool
}

// resolveNamed follows a written type name, local `T` or qualified `pkg.T`,
// through aliases to the named type that declares it. Qualifiers are read in
// the output's scope.
func (u *unit) resolveNamed(written string) (namedStruct, error) {
	expr, err := parser.ParseExpr(written)
	if err != nil {
		return namedStruct{}, fmt.Errorf("type %q: %w", written, err)
	}
	src := u.pkg
	var name string
	switch e := expr.(type) {
	case *ast.Ident:
		name = e.Name
	case *ast.SelectorExpr:
		qualifier, ok := e.X.(*ast.Ident)
		if !ok {
			return namedStruct{}, fmt.Errorf("type %s is not a named type", written)
		}
		importPath := u.scope(qualifier.Name)
		if importPath == "" {
			return namedStruct{}, fmt.Errorf("qualifier %s has no unambiguous import in package %s", qualifier.Name, u.pkg.path)
		}
		if src, err = u.loader.loadImport(importPath); err != nil {
			return namedStruct{}, err
		}
		name = e.Sel.Name
	default:
		return namedStruct{}, fmt.Errorf("type %s is not a named type", written)
	}
	spec := src.types[name]
	if spec == nil || !spec.Assign.IsValid() {
		return namedStruct{src: src, name: name}, nil
	}
	rsrc, _, end := u.loader.unalias(src, src.owner[spec], spec.Type)
	ident, ok := end.(*ast.Ident)
	if !ok || rsrc.types[ident.Name] == nil {
		return namedStruct{}, fmt.Errorf("type %s is an alias of %s, not a named type declared in module %s", written, u.loader.text(end), u.loader.module)
	}
	return namedStruct{src: rsrc, name: ident.Name, aliased: true}, nil
}

// scope maps a qualifier of a type written for the output to its import
// path: the output's imports, qualifiers chosen for foreign structs, then
// the output package's own imports.
func (u *unit) scope(qualifier string) string {
	if importPath := u.imports[qualifier]; importPath != "" {
		return importPath
	}
	for src, name := range u.foreign {
		if name == qualifier {
			return src.path
		}
	}
	return u.pkg.packageImports()[qualifier]
}

// sameNamed reports whether two written type names resolve to one declared
// type, so a target named through an alias matches its struct's spelling.
func (u *unit) sameNamed(a, b string) bool {
	left, err := u.resolveNamed(a)
	if err != nil {
		return false
	}
	right, err := u.resolveNamed(b)
	if err != nil {
		return false
	}
	return left.src == right.src && left.name == right.name && left.src.types[left.name] != nil
}

// qualifierOf names a foreign package in the output: the qualifier already
// used for it, its import in the output package, or a fresh unused name. The
// name is imported only when a printed type uses it.
func (u *unit) qualifierOf(src *source) string {
	if name := u.foreign[src]; name != "" {
		return name
	}
	name := ""
	for _, imported := range sortedKeys(u.imports) {
		if u.imports[imported] == src.path {
			name = imported
			break
		}
	}
	if name == "" {
		imports := u.pkg.packageImports()
		for _, imported := range sortedKeys(imports) {
			if imports[imported] == src.path && identifier(imported) && imported != "_" {
				name = imported
				break
			}
		}
	}
	if name == "" {
		base := defaultName(src.path)
		reserved := u.reservedNames()
		name = base
		for i := 2; reserved[name] || u.scope(name) != ""; i++ {
			name = fmt.Sprintf("%s%d", base, i)
		}
	}
	if u.foreign == nil {
		u.foreign = map[*source]string{}
	}
	u.foreign[src] = name
	return name
}
