package gqlx

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Lexer tokenizes a GraphQL source document.
type Lexer struct {
	source string
	pos    int
	line   int
	col    int
}

// NewLexer creates a new Lexer for the given source.
func NewLexer(source string) *Lexer {
	return &Lexer{source: source, pos: 0, line: 1, col: 1}
}

// ReadAllTokens returns all tokens from the source.
func (l *Lexer) ReadAllTokens() ([]Token, error) {
	var tokens []Token
	for {
		tok, err := l.NextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Kind == TokenEOF {
			break
		}
	}
	return tokens, nil
}

// NextToken reads the next token from the source.
func (l *Lexer) NextToken() (Token, error) {
	l.skipIgnored()

	if l.pos >= len(l.source) {
		return Token{Kind: TokenEOF, Line: l.line, Col: l.col}, nil
	}

	ch := l.source[l.pos]

	// Punctuators
	switch ch {
	case '!':
		return l.singleCharToken(TokenBang), nil
	case '$':
		return l.singleCharToken(TokenDollar), nil
	case '&':
		return l.singleCharToken(TokenAmp), nil
	case '(':
		return l.singleCharToken(TokenParenL), nil
	case ')':
		return l.singleCharToken(TokenParenR), nil
	case ':':
		return l.singleCharToken(TokenColon), nil
	case '=':
		return l.singleCharToken(TokenEquals), nil
	case '@':
		return l.singleCharToken(TokenAt), nil
	case '[':
		return l.singleCharToken(TokenBracketL), nil
	case ']':
		return l.singleCharToken(TokenBracketR), nil
	case '{':
		return l.singleCharToken(TokenBraceL), nil
	case '|':
		return l.singleCharToken(TokenPipe), nil
	case '}':
		return l.singleCharToken(TokenBraceR), nil
	case '.':
		if l.pos+2 < len(l.source) && l.source[l.pos+1] == '.' && l.source[l.pos+2] == '.' {
			tok := Token{Kind: TokenSpread, Value: "...", Line: l.line, Col: l.col}
			l.advance(3)
			return tok, nil
		}
		return Token{}, l.syntaxError("unexpected character '.'")
	case '"':
		if l.pos+2 < len(l.source) && l.source[l.pos+1] == '"' && l.source[l.pos+2] == '"' {
			return l.readBlockString()
		}
		return l.readString()
	}

	// Name or keyword
	if isNameStart(ch) {
		return l.readName(), nil
	}

	// Number (Int or Float)
	if ch == '-' || isDigit(ch) {
		return l.readNumber()
	}

	return Token{}, l.syntaxError(fmt.Sprintf("unexpected character %q", ch))
}

func (l *Lexer) singleCharToken(kind TokenKind) Token {
	tok := Token{Kind: kind, Value: string(l.source[l.pos]), Line: l.line, Col: l.col}
	l.advance(1)
	return tok
}

func (l *Lexer) advance(n int) {
	for i := 0; i < n && l.pos < len(l.source); i++ {
		if l.source[l.pos] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.pos++
	}
}

func (l *Lexer) skipIgnored() {
	for l.pos < len(l.source) {
		ch := l.source[l.pos]
		switch {
		case ch == '\xEF' && l.pos+2 < len(l.source) && l.source[l.pos+1] == '\xBB' && l.source[l.pos+2] == '\xBF':
			// BOM
			l.advance(3)
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == ',':
			l.advance(1)
		case ch == '#':
			l.skipComment()
		default:
			return
		}
	}
}

func (l *Lexer) skipComment() {
	// Skip from '#' to end of line
	l.advance(1) // skip '#'
	for l.pos < len(l.source) && l.source[l.pos] != '\n' && l.source[l.pos] != '\r' {
		l.advance(1)
	}
}

func (l *Lexer) readName() Token {
	startLine, startCol := l.line, l.col
	start := l.pos
	for l.pos < len(l.source) && isNameContinue(l.source[l.pos]) {
		l.pos++
		l.col++
	}
	return Token{Kind: TokenName, Value: l.source[start:l.pos], Line: startLine, Col: startCol}
}

func (l *Lexer) readNumber() (Token, error) {
	startLine, startCol := l.line, l.col
	start := l.pos
	isFloat := false

	// Optional negative
	if l.pos < len(l.source) && l.source[l.pos] == '-' {
		l.pos++
		l.col++
	}

	// Integer part
	if l.pos < len(l.source) && l.source[l.pos] == '0' {
		l.pos++
		l.col++
	} else if l.pos < len(l.source) && isDigit(l.source[l.pos]) {
		for l.pos < len(l.source) && isDigit(l.source[l.pos]) {
			l.pos++
			l.col++
		}
	} else {
		return Token{}, l.syntaxError("invalid number, expected digit")
	}

	// Fractional part
	if l.pos < len(l.source) && l.source[l.pos] == '.' {
		isFloat = true
		l.pos++
		l.col++
		if l.pos >= len(l.source) || !isDigit(l.source[l.pos]) {
			return Token{}, l.syntaxError("invalid number, expected digit after '.'")
		}
		for l.pos < len(l.source) && isDigit(l.source[l.pos]) {
			l.pos++
			l.col++
		}
	}

	// Exponent part
	if l.pos < len(l.source) && (l.source[l.pos] == 'e' || l.source[l.pos] == 'E') {
		isFloat = true
		l.pos++
		l.col++
		if l.pos < len(l.source) && (l.source[l.pos] == '+' || l.source[l.pos] == '-') {
			l.pos++
			l.col++
		}
		if l.pos >= len(l.source) || !isDigit(l.source[l.pos]) {
			return Token{}, l.syntaxError("invalid number, expected digit in exponent")
		}
		for l.pos < len(l.source) && isDigit(l.source[l.pos]) {
			l.pos++
			l.col++
		}
	}

	kind := TokenInt
	if isFloat {
		kind = TokenFloat
	}
	return Token{Kind: TokenKind(kind), Value: l.source[start:l.pos], Line: startLine, Col: startCol}, nil
}

func (l *Lexer) readString() (Token, error) {
	startLine, startCol := l.line, l.col
	l.pos++ // skip opening '"'
	l.col++

	var sb strings.Builder
	for {
		if l.pos >= len(l.source) {
			return Token{}, l.syntaxError("unterminated string")
		}
		ch := l.source[l.pos]
		if ch == '\n' || ch == '\r' {
			return Token{}, l.syntaxError("unterminated string")
		}
		if ch == '"' {
			l.pos++
			l.col++
			return Token{Kind: TokenString, Value: sb.String(), Line: startLine, Col: startCol}, nil
		}
		if ch == '\\' {
			l.pos++
			l.col++
			if l.pos >= len(l.source) {
				return Token{}, l.syntaxError("unterminated string")
			}
			escaped := l.source[l.pos]
			switch escaped {
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			case '/':
				sb.WriteByte('/')
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				r, err := l.readUnicodeEscape()
				if err != nil {
					return Token{}, err
				}
				var buf [4]byte
				n := utf8.EncodeRune(buf[:], r)
				sb.Write(buf[:n])
			default:
				return Token{}, l.syntaxError(fmt.Sprintf("invalid escape character: %q", escaped))
			}
			l.pos++
			l.col++
			continue
		}
		sb.WriteByte(ch)
		l.pos++
		l.col++
	}
}

func (l *Lexer) readUnicodeEscape() (rune, error) {
	l.pos++ // skip 'u'
	l.col++
	if l.pos+4 > len(l.source) {
		return 0, l.syntaxError("invalid unicode escape sequence")
	}
	hex := l.source[l.pos : l.pos+4]
	var r rune
	for _, c := range hex {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, l.syntaxError(fmt.Sprintf("invalid unicode escape character: %q", c))
		}
	}
	l.pos += 3 // advance 3, the caller will advance 1 more
	l.col += 3
	return r, nil
}

func (l *Lexer) readBlockString() (Token, error) {
	startLine, startCol := l.line, l.col
	l.advance(3) // skip opening """

	var sb strings.Builder
	for {
		if l.pos >= len(l.source) {
			return Token{}, l.syntaxError("unterminated block string")
		}
		if l.source[l.pos] == '"' && l.pos+2 < len(l.source) && l.source[l.pos+1] == '"' && l.source[l.pos+2] == '"' {
			val := blockStringValue(sb.String())
			l.advance(3) // skip closing """
			return Token{Kind: TokenBlockString, Value: val, Line: startLine, Col: startCol}, nil
		}
		if l.source[l.pos] == '\\' && l.pos+3 < len(l.source) && l.source[l.pos+1] == '"' && l.source[l.pos+2] == '"' && l.source[l.pos+3] == '"' {
			sb.WriteString(`"""`)
			l.advance(4)
			continue
		}
		sb.WriteByte(l.source[l.pos])
		l.advance(1)
	}
}

// blockStringValue implements the BlockStringValue algorithm from the spec.
func blockStringValue(raw string) string {
	lines := strings.Split(raw, "\n")
	// Normalize \r\n and \r
	var normalized []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		normalized = append(normalized, line)
	}
	lines = normalized

	// Determine common indent
	commonIndent := -1
	for i, line := range lines {
		if i == 0 {
			continue
		}
		indent := leadingWhitespace(line)
		if indent < len(line) {
			if commonIndent == -1 || indent < commonIndent {
				commonIndent = indent
			}
		}
	}

	// Remove common indent
	if commonIndent > 0 {
		for i := 1; i < len(lines); i++ {
			if len(lines[i]) >= commonIndent {
				lines[i] = lines[i][commonIndent:]
			}
		}
	}

	// Remove leading blank lines
	for len(lines) > 0 && isBlankLine(lines[0]) {
		lines = lines[1:]
	}

	// Remove trailing blank lines
	for len(lines) > 0 && isBlankLine(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func leadingWhitespace(s string) int {
	for i, ch := range s {
		if ch != ' ' && ch != '\t' {
			return i
		}
	}
	return len(s)
}

func isBlankLine(s string) bool {
	return leadingWhitespace(s) == len(s)
}

func (l *Lexer) syntaxError(msg string) error {
	return fmt.Errorf("syntax error at %d:%d: %s", l.line, l.col, msg)
}

func isNameStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isNameContinue(ch byte) bool {
	return isNameStart(ch) || isDigit(ch)
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
