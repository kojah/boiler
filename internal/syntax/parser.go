package syntax

import (
	"fmt"
	"text/scanner"
)

type parser struct {
	tokens []lexeme
	at     int
}

// Parse reads the complete .boiler language from memory. Errors carry source
// positions. Duplicate names and recipe-specific meaning are checked later.
func Parse(filename, source string) (*File, error) {
	tokens, err := scan(filename, source)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	file := &File{}
	for p.breaks(); p.peek().kind != scanner.EOF; p.breaks() {
		if err := p.declaration(file); err != nil {
			return nil, err
		}
	}

	return file, nil
}

func (p *parser) declaration(file *File) error {

	token := p.peek()
	p.at++
	switch token.text {
	case "recipe":
		name, err := p.name()
		if err != nil {
			return err
		}
		if err := p.expect('='); err != nil {
			return err
		}
		path, err := p.string()
		if err != nil {
			return err
		}
		file.Recipes = append(file.Recipes, Recipe{Name: name, Path: path, Pos: token.pos})
		if err := p.endLine(); err != nil {
			return err
		}
	case "readonly":
		if err := p.expect('='); err != nil {
			return err
		}
		value, err := p.string()
		if err != nil {
			return err
		}
		file.Settings = append(file.Settings, Setting{Name: token.text, Value: value, Pos: token.pos})
		if err := p.endLine(); err != nil {
			return err
		}
	case "output":
		output, err := p.output(token.pos)
		if err != nil {
			return err
		}
		file.Blocks = append(file.Blocks, output)
	case "package":
		pkg, err := p.packageBlock(token.pos)
		if err != nil {
			return err
		}
		file.Blocks = append(file.Blocks, pkg)
	default:
		return p.errorAt(token, "expected recipe, readonly, output or package declaration")
	}
	return nil
}

func (p *parser) peek() lexeme { return p.tokens[p.at] }
func (p *parser) breaks() {
	for p.peek().kind == '\n' || p.peek().kind == ';' {
		p.at++
	}
}
func (p *parser) errorAt(token lexeme, message string) error {
	return fmt.Errorf("%s: %s (found %q)", token.pos, message, token.text)
}
func (p *parser) expect(kind rune) error {
	if p.peek().kind != kind {
		return p.errorAt(p.peek(), fmt.Sprintf("expected %q", kind))
	}
	p.at++
	return nil
}
func (p *parser) name() (string, error) {
	token := p.peek()
	if token.kind != scanner.Ident {
		return "", p.errorAt(token, "expected identifier")
	}
	p.at++
	return token.text, nil
}
func (p *parser) string() (string, error) {
	value, err := stringValue(p.peek())
	if err != nil {
		return "", err
	}
	p.at++
	return value, nil
}
func (p *parser) endLine() error {
	switch p.peek().kind {
	case '\n', ';', scanner.EOF:
		return nil
	default:
		return p.errorAt(p.peek(), "expected newline or semicolon")
	}
}

func (p *parser) output(pos Position) (Output, error) {
	path, err := p.string()
	if err != nil {
		return Output{}, err
	}
	result := Output{Path: path, Pos: pos}
	p.breaks()
	if p.peek().text == "build" {
		p.at++
		if err := p.expect('('); err != nil {
			return result, err
		}
		result.Build, err = p.string()
		if err != nil {
			return result, err
		}
		if result.Build == "" {
			return result, fmt.Errorf("%s: build constraint must not be empty", pos)
		}
		if err := p.expect(')'); err != nil {
			return result, err
		}
		p.breaks()
	}
	if err := p.expect('{'); err != nil {
		return result, err
	}
	for p.breaks(); p.peek().kind != '}'; p.breaks() {
		target, err := p.target()
		if err != nil {
			return result, err
		}
		result.Targets = append(result.Targets, target)
	}
	p.at++
	return result, nil
}

func (p *parser) packageBlock(pos Position) (Package, error) {
	path, err := p.string()
	if err != nil {
		return Package{}, err
	}
	result := Package{Path: path, Pos: pos}
	p.breaks()
	if err := p.expect('{'); err != nil {
		return result, err
	}
	for p.breaks(); p.peek().kind != '}'; p.breaks() {
		token := p.peek()
		if token.kind != scanner.Ident || token.text != "output" {
			return result, p.errorAt(token, "expected output declaration in package")
		}
		p.at++
		output, err := p.output(token.pos)
		if err != nil {
			return result, err
		}
		result.Outputs = append(result.Outputs, output)
	}
	p.at++
	return result, nil
}
