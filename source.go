package boiler

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// shape is the syntactic container kind of a field type.
type shape int

const (
	otherShape shape = iota
	pointerShape
	sliceShape
	mapShape
)

// field is one struct field as written. Embedded fields take the base name
// of their type.
type field struct {
	name string
	typ  ast.Expr
	file *ast.File
}

// source is the parsed syntax of one package directory. Test files are
// skipped; generated files are kept because they can declare types.
type source struct {
	path  string
	files []*ast.File
	types map[string]*ast.TypeSpec
	owner map[*ast.TypeSpec]*ast.File
}

// loader parses module packages on demand, keyed by import path.
type loader struct {
	moduleRoot, module string
	fset               *token.FileSet
	packages           map[string]*source
}

func newLoader(moduleRoot, module string) *loader {
	return &loader{moduleRoot: moduleRoot, module: module, fset: token.NewFileSet(), packages: map[string]*source{}}
}

// load parses the package in dir, an absolute directory inside the module.
func (l *loader) load(dir string) (*source, error) {
	rel, err := filepath.Rel(l.moduleRoot, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("%s is outside module %s", dir, l.module)
	}
	importPath := path.Join(l.module, filepath.ToSlash(rel))
	if loaded := l.packages[importPath]; loaded != nil {
		return loaded, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	src := &source{path: importPath, types: map[string]*ast.TypeSpec{}, owner: map[*ast.TypeSpec]*ast.File{}}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || outputName.MatchString(name) {
			continue
		}
		file, err := parser.ParseFile(l.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		src.files = append(src.files, file)
		src.collectTypes(file)
	}
	l.packages[importPath] = src
	return src, nil
}

func (s *source) collectTypes(file *ast.File) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			s.types[typeSpec.Name.Name] = typeSpec
			s.owner[typeSpec] = file
		}
	}
}

// loadImport parses a module package by import path. Packages outside the
// module are unreadable without loading the build, so they are errors.
func (l *loader) loadImport(importPath string) (*source, error) {
	rel, ok := strings.CutPrefix(importPath, l.module+"/")
	if !ok {
		return nil, fmt.Errorf("package %s is outside module %s", importPath, l.module)
	}
	return l.load(filepath.Join(l.moduleRoot, filepath.FromSlash(rel)))
}

// structFields returns the fields of a struct type declared in s.
func (s *source) structFields(name string) ([]field, error) {
	spec := s.types[name]
	if spec == nil {
		return nil, fmt.Errorf("type %s is not declared in %s", name, s.path)
	}
	if spec.TypeParams != nil {
		return nil, fmt.Errorf("type %s is generic; boiler supports only non-generic structs", name)
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok || spec.Assign.IsValid() {
		return nil, fmt.Errorf("type %s is not a struct declaration", name)
	}
	var fields []field
	for _, list := range st.Fields.List {
		if len(list.Names) == 0 {
			fields = append(fields, field{name: embeddedName(list.Type), typ: list.Type, file: s.owner[spec]})
			continue
		}
		for _, ident := range list.Names {
			if ident.Name != "_" {
				fields = append(fields, field{name: ident.Name, typ: list.Type, file: s.owner[spec]})
			}
		}
	}
	return fields, nil
}

func embeddedName(expr ast.Expr) string {
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		case *ast.SelectorExpr:
			return e.Sel.Name
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// shapeOf classifies a field type and returns its element and key types. A
// local named type is followed one level to its declared container.
func (s *source) shapeOf(expr ast.Expr) (shape, ast.Expr, ast.Expr) {
	if ident, ok := expr.(*ast.Ident); ok {
		if spec := s.types[ident.Name]; spec != nil && spec.TypeParams == nil {
			expr = spec.Type
		}
	}
	switch e := expr.(type) {
	case *ast.StarExpr:
		return pointerShape, e.X, nil
	case *ast.ArrayType:
		if e.Len == nil {
			return sliceShape, e.Elt, nil
		}
	case *ast.MapType:
		return mapShape, e.Value, e.Key
	}
	return otherShape, nil, nil
}

// importsOf maps each qualifier a file can use to its import path. Unnamed
// imports use the last path element without a major-version suffix.
func importsOf(file *ast.File) map[string]string {
	result := map[string]string{}
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := defaultName(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		result[name] = importPath
	}
	return result
}

func defaultName(importPath string) string {
	parts := strings.Split(importPath, "/")
	last := parts[len(parts)-1]
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = parts[len(parts)-2]
	}
	return last
}

// packageImports merges the qualifiers of every file in s. A qualifier bound
// to two paths is ambiguous and maps to the empty string.
func (s *source) packageImports() map[string]string {
	result := map[string]string{}
	for _, file := range s.files {
		for name, importPath := range importsOf(file) {
			if previous, ok := result[name]; ok && previous != importPath {
				importPath = ""
			}
			result[name] = importPath
		}
	}
	return result
}

// text prints a type expression as written.
func (l *loader) text(expr ast.Expr) string {
	var out bytes.Buffer
	if err := printer.Fprint(&out, l.fset, expr); err != nil {
		return fmt.Sprintf("<%v>", err)
	}
	return out.String()
}

// qualifiers lists the package qualifiers an expression uses.
func qualifiers(expr ast.Expr) []string {
	seen := map[string]bool{}
	ast.Inspect(expr, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				seen[ident.Name] = true
			}
			return false
		}
		return true
	})
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
