package boiler

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"go.yaml.in/yaml/v3"
)

func readYAML(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeYAML(path, data, value)
}

func decodeYAML(path string, data []byte, value any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("%s: expected one YAML document", path)
	}
	return nil
}
