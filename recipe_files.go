package boiler

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Standard recipes are ordinary declarative packages bundled for portable use.
//
//go:embed recipes/*/*.yaml recipes/*/*.tmpl
var standardRecipes embed.FS

func (u *unit) recipeFiles(name string) (fs.FS, error) {
	reference := u.config.Recipes[name]
	if name, bundled := strings.CutPrefix(reference, "boiler:"); bundled {
		if !identifier(name) {
			return nil, fmt.Errorf("invalid bundled recipe %q", reference)
		}
		files, err := fs.Sub(standardRecipes, "recipes/"+name)
		if err != nil {
			return nil, fmt.Errorf("recipe %s: %w", reference, err)
		}
		return files, nil
	}
	dir := filepath.Join(u.config.root, reference)
	rel, err := filepath.Rel(u.config.moduleRoot, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("recipe %s must be inside the module", name)
	}
	return os.DirFS(dir), nil
}

func readRecipeYAML(files fs.FS, path string, value any) error {
	data, err := fs.ReadFile(files, filepath.ToSlash(path))
	if err != nil {
		return err
	}
	return decodeYAML(path, data, value)
}
