package boiler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Publish writes artifacts, or with check reports stale or missing outputs
// without writing. A file at an output path must carry the boiler header, so
// hand-written code is never overwritten. When every output was compiled,
// boiler_*_gen.go files in target packages that no output produces are
// obsolete: check reports them and publication removes them. Packages that
// no longer hold any output are not scanned. Each file is replaced
// atomically; the batch is not transactional.
func (c *Config) Publish(artifacts []Artifact, check bool) error {
	var problems []string
	produced := map[string]bool{}
	for _, a := range artifacts {
		produced[a.Path] = true
		old, err := os.ReadFile(a.Path)
		switch {
		case os.IsNotExist(err):
			problems = append(problems, "missing "+c.rel(a.Path))
		case err != nil:
			return err
		case !bytes.HasPrefix(old, []byte(header)):
			return fmt.Errorf("%s exists without the boiler header; refusing to overwrite", c.rel(a.Path))
		case !bytes.Equal(old, a.Data):
			problems = append(problems, "stale "+c.rel(a.Path))
		default:
			continue
		}
		if !check {
			if err := writeAtomic(a.Path, a.Data); err != nil {
				return err
			}
		}
	}
	if len(artifacts) == len(c.Outputs) {
		obsolete, err := c.obsolete(produced)
		if err != nil {
			return err
		}
		for _, file := range obsolete {
			problems = append(problems, "obsolete "+c.rel(file))
			if !check {
				if err := os.Remove(file); err != nil {
					return err
				}
			}
		}
	}
	if check && len(problems) > 0 {
		return fmt.Errorf("boiler outputs are not current (run boiler generate):\n%s", strings.Join(problems, "\n"))
	}
	return nil
}

func (c *Config) rel(path string) string {
	if rel, err := filepath.Rel(c.root, path); err == nil {
		return rel
	}
	return path
}

// obsolete lists boiler outputs in target packages that no artifact produced.
func (c *Config) obsolete(produced map[string]bool) ([]string, error) {
	dirs := map[string]bool{}
	for _, t := range c.Outputs {
		dirs[filepath.Join(c.root, t.Package)] = true
	}
	var result []string
	for dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			file := filepath.Join(dir, entry.Name())
			if entry.IsDir() || !outputName.MatchString(entry.Name()) || produced[file] {
				continue
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			if bytes.HasPrefix(data, []byte(header)) {
				result = append(result, file)
			}
		}
	}
	sort.Strings(result)
	return result, nil
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".boiler-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(0o644)
	}
	// Close after a failed write only releases the descriptor; the write
	// error is the one to report.
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
