package syntax

import "text/scanner"

func (p *parser) rule(depth int) (*Rule, error) {
	if depth > 64 {
		return nil, p.errorAt(p.peek(), "rule nesting exceeds 64")
	}
	rule := &Rule{Pos: p.peek().pos}
	var err error
	if rule.Name, err = p.name(); err != nil {
		return nil, err
	}
	if p.peek().kind == '(' {
		p.at++
		p.breaks()
		for {
			child, err := p.rule(depth + 1)
			if err != nil {
				return nil, err
			}
			rule.Children = append(rule.Children, child)
			p.breaks()
			if p.peek().kind == ')' {
				p.at++
				return rule, nil
			}
			if p.peek().kind != ',' {
				return nil, p.errorAt(p.peek(), "expected , or ) after nested rule")
			}
			p.at++
			p.breaks()
		}
	}
	for p.peek().kind == scanner.Ident || p.peek().kind == '*' {
		start := p.at
		if p.peek().kind == '*' {
			p.at++
		}
		if _, err := p.name(); err != nil {
			return nil, err
		}
		for p.peek().kind == '.' {
			p.at++
			if _, err := p.name(); err != nil {
				return nil, err
			}
		}
		rule.Args = append(rule.Args, written(p.tokens[start:p.at]))
	}
	return rule, nil
}
