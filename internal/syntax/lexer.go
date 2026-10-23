package syntax

import (
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
)

type lexeme struct {
	kind rune
	text string
	pos  Position
	end  int
}

func scan(filename, source string) ([]lexeme, error) {
	var lexer scanner.Scanner
	lexer.Init(strings.NewReader(source))
	lexer.Filename = filename
	lexer.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanRawStrings | scanner.ScanComments | scanner.SkipComments
	lexer.Whitespace = 1<<' ' | 1<<'\t' | 1<<'\r'
	var first error
	lexer.Error = func(s *scanner.Scanner, message string) {
		if first == nil {
			first = fmt.Errorf("%s: %s", s.Position, message)
		}
	}
	var tokens []lexeme
	for {
		kind := lexer.Scan()
		if first != nil {
			return nil, first
		}
		pos := Position{filename, lexer.Offset, lexer.Line, lexer.Column}
		text := lexer.TokenText()
		tokens = append(tokens, lexeme{kind: kind, text: text, pos: pos, end: pos.Offset + len(text)})
		if kind == scanner.EOF {
			return tokens, nil
		}
	}
}

// String renders a position for source diagnostics.
func (pos Position) String() string {
	return fmt.Sprintf("%s:%d:%d", pos.Filename, pos.Line, pos.Column)
}

func stringValue(token lexeme) (string, error) {
	if token.kind != scanner.String && token.kind != scanner.RawString {
		return "", fmt.Errorf("%s: expected quoted string", token.pos)
	}
	value, err := strconv.Unquote(token.text)
	if err != nil {
		return "", fmt.Errorf("%s: %w", token.pos, err)
	}
	return value, nil
}

// written joins tokens while retaining lexical separation. Comments are
// whitespace; their contents never become part of a type or option value.
func written(tokens []lexeme) string {
	var out strings.Builder
	end := -1
	for _, token := range tokens {
		if token.kind == '\n' {
			continue
		}
		if end >= 0 && end < token.pos.Offset {
			out.WriteByte(' ')
		}
		out.WriteString(token.text)
		end = token.end
	}
	return strings.TrimSpace(out.String())
}
