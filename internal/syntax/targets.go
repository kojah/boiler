package syntax

import (
	"text/scanner"
)

func (p *parser) target() (Target, error) {
	target := Target{Pos: p.peek().pos}
	var err error
	if target.Recipe, err = p.name(); err != nil {
		return target, err
	}
	if target.Name, err = p.name(); err != nil {
		return target, err
	}
	args, err := p.arguments()
	if err != nil {
		return target, err
	}
	if len(args) != 1 || args[0].Text == "" {
		return target, p.errorAt(p.peek(), "target requires one source type")
	}
	target.Type = args[0].Text
	for p.breaks(); p.peek().kind != '{'; p.breaks() {
		option := Option{Pos: p.peek().pos}
		if option.Name, err = p.name(); err != nil {
			return target, err
		}
		if option.Arguments, err = p.arguments(); err != nil {
			return target, err
		}
		target.Options = append(target.Options, option)
	}
	p.at++
	for p.breaks(); p.peek().kind != '}'; p.breaks() {
		group, err := p.fields()
		if err != nil {
			return target, err
		}
		target.Fields = append(target.Fields, group)
	}
	p.at++
	return target, nil
}

func (p *parser) fields() (FieldGroup, error) {
	group := FieldGroup{Pos: p.peek().pos}
	for {
		name, err := p.name()
		if err != nil {
			return group, err
		}
		group.Names = append(group.Names, name)
		if p.peek().kind != ',' {
			break
		}
		p.at++
		p.breaks()
	}
	if err := p.expect('='); err != nil {
		return group, err
	}
	rule, err := p.rule(0)
	if err != nil {
		return group, err
	}
	group.Rule = rule
	if p.peek().kind != '}' {
		if err := p.endLine(); err != nil {
			return group, err
		}
	}
	return group, nil
}

// arguments splits only top-level commas, retaining brackets and parentheses
// inside Go type expressions. The recipe schema later interprets each value.
func (p *parser) arguments() ([]Argument, error) {
	if err := p.expect('('); err != nil {
		return nil, err
	}
	p.breaks()
	var args []Argument
	if p.peek().kind == ')' {
		p.at++
		return args, nil
	}
	for {
		arg, err := p.argument()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.peek().kind == ')' {
			p.at++
			return args, nil
		}
		if err := p.expect(','); err != nil {
			return nil, err
		}
		p.breaks()
		if p.peek().kind == ')' {
			return nil, p.errorAt(p.peek(), "expected argument after comma")
		}
	}
}

func (p *parser) argument() (Argument, error) {
	start := p.at
	arg := Argument{Pos: p.peek().pos}
	var stack []rune
	for {
		token := p.peek()
		if len(stack) == 0 && (token.kind == ',' || token.kind == ')') {
			break
		}
		switch token.kind {
		case scanner.EOF:
			return arg, p.errorAt(token, "unterminated argument list")
		case '(', '[', '{':
			if len(stack) >= 64 {
				return arg, p.errorAt(token, "argument nesting exceeds 64")
			}
			stack = append(stack, map[rune]rune{'(': ')', '[': ']', '{': '}'}[token.kind])
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != token.kind {
				return arg, p.errorAt(token, "unmatched delimiter")
			}
			stack = stack[:len(stack)-1]
		}
		p.at++
	}
	tokens := p.tokens[start:p.at]
	for len(tokens) > 0 && tokens[len(tokens)-1].kind == '\n' {
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) == 0 {
		return arg, p.errorAt(p.peek(), "expected argument")
	}
	if len(tokens) == 1 && (tokens[0].kind == scanner.String || tokens[0].kind == scanner.RawString) {
		value, err := stringValue(tokens[0])
		arg.Text = value
		arg.Quoted = true
		return arg, err
	}
	arg.Text = written(tokens)
	return arg, nil
}
