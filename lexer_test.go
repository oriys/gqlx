package gqlx

import (
	"testing"
)

func TestLexerPunctuators(t *testing.T) {
	source := "! $ & ( ) ... : = @ [ ] { | }"
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	expected := []TokenKind{
		TokenBang, TokenDollar, TokenAmp, TokenParenL, TokenParenR,
		TokenSpread, TokenColon, TokenEquals, TokenAt,
		TokenBracketL, TokenBracketR, TokenBraceL, TokenPipe, TokenBraceR,
		TokenEOF,
	}
	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(expected))
	}
	for i, tok := range tokens {
		if tok.Kind != expected[i] {
			t.Errorf("token[%d] = %s, want %s", i, tok.Kind, expected[i])
		}
	}
}

func TestLexerNames(t *testing.T) {
	source := "query mutation fragment on _private camelCase"
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"query", "mutation", "fragment", "on", "_private", "camelCase"}
	for i, name := range names {
		if tokens[i].Kind != TokenName || tokens[i].Value != name {
			t.Errorf("token[%d] = {%s, %q}, want {Name, %q}", i, tokens[i].Kind, tokens[i].Value, name)
		}
	}
}

func TestLexerIntegers(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{"0", "0"},
		{"42", "42"},
		{"-1", "-1"},
		{"1234567890", "1234567890"},
	}
	for _, tt := range tests {
		tokens, err := NewLexer(tt.input).ReadAllTokens()
		if err != nil {
			t.Errorf("Lexer(%q) error: %v", tt.input, err)
			continue
		}
		if tokens[0].Kind != TokenInt || tokens[0].Value != tt.value {
			t.Errorf("Lexer(%q) = {%s, %q}, want {Int, %q}", tt.input, tokens[0].Kind, tokens[0].Value, tt.value)
		}
	}
}

func TestLexerFloats(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{"1.0", "1.0"},
		{"-3.14", "-3.14"},
		{"1e10", "1e10"},
		{"1E10", "1E10"},
		{"1.5e+3", "1.5e+3"},
		{"1.5e-3", "1.5e-3"},
		{"0.123", "0.123"},
	}
	for _, tt := range tests {
		tokens, err := NewLexer(tt.input).ReadAllTokens()
		if err != nil {
			t.Errorf("Lexer(%q) error: %v", tt.input, err)
			continue
		}
		if tokens[0].Kind != TokenFloat || tokens[0].Value != tt.value {
			t.Errorf("Lexer(%q) = {%s, %q}, want {Float, %q}", tt.input, tokens[0].Kind, tokens[0].Value, tt.value)
		}
	}
}

func TestLexerStrings(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{`"hello"`, "hello"},
		{`"hello world"`, "hello world"},
		{`"escaped \"quotes\""`, `escaped "quotes"`},
		{`"tab\there"`, "tab\there"},
		{`"newline\nhere"`, "newline\nhere"},
		{`"backslash\\"`, "backslash\\"},
		{`"slash\/"`, "slash/"},
		{`"unicode\u0041"`, "unicodeA"},
	}
	for _, tt := range tests {
		tokens, err := NewLexer(tt.input).ReadAllTokens()
		if err != nil {
			t.Errorf("Lexer(%q) error: %v", tt.input, err)
			continue
		}
		if tokens[0].Kind != TokenString || tokens[0].Value != tt.value {
			t.Errorf("Lexer(%q) = {%s, %q}, want {String, %q}", tt.input, tokens[0].Kind, tokens[0].Value, tt.value)
		}
	}
}

func TestLexerBlockStrings(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{`"""hello"""`, "hello"},
		{"\"\"\"hello\nworld\"\"\"", "hello\nworld"},
		{"\"\"\"  hello\n  world\"\"\"", "  hello\nworld"},
		{`"""contains \"""triple quotes\""" inside"""`, `contains """triple quotes""" inside`},
	}
	for _, tt := range tests {
		tokens, err := NewLexer(tt.input).ReadAllTokens()
		if err != nil {
			t.Errorf("Lexer(%q) error: %v", tt.input, err)
			continue
		}
		if tokens[0].Kind != TokenBlockString || tokens[0].Value != tt.value {
			t.Errorf("Lexer(%q) = {%s, %q}, want {BlockString, %q}", tt.input, tokens[0].Kind, tokens[0].Value, tt.value)
		}
	}
}

func TestLexerComments(t *testing.T) {
	source := "# comment\nname"
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Kind != TokenName || tokens[0].Value != "name" {
		t.Errorf("expected Name 'name' after comment, got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestLexerIgnoresWhitespaceAndCommas(t *testing.T) {
	source := " \t\n\r , name , "
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[0].Kind != TokenName {
		t.Errorf("expected 2 tokens (Name + EOF), got %d", len(tokens))
	}
}

func TestLexerBOM(t *testing.T) {
	source := "\xEF\xBB\xBFname"
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Kind != TokenName || tokens[0].Value != "name" {
		t.Errorf("expected Name 'name' after BOM, got %s %q", tokens[0].Kind, tokens[0].Value)
	}
}

func TestLexerLineAndColumn(t *testing.T) {
	source := "query\n  name"
	tokens, err := NewLexer(source).ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	if tokens[0].Line != 1 || tokens[0].Col != 1 {
		t.Errorf("first token at %d:%d, want 1:1", tokens[0].Line, tokens[0].Col)
	}
	if tokens[1].Line != 2 || tokens[1].Col != 3 {
		t.Errorf("second token at %d:%d, want 2:3", tokens[1].Line, tokens[1].Col)
	}
}

func TestLexerErrors(t *testing.T) {
	tests := []string{
		".",          // unexpected character
		"..",         // unexpected character
		`"unterminated`, // unterminated string
		"\"new\nline\"", // unterminated string (newline in string)
		"-",          // invalid number
		"-.5",        // invalid number
		"1.",         // invalid number, digit after dot
		"1e",         // invalid number, digit in exponent
		`"\x"`,       // invalid escape
		`"\uXXXX"`,   // invalid unicode
		"\x80",       // unexpected character (non-ASCII non-name-start)
	}
	for _, input := range tests {
		_, err := NewLexer(input).ReadAllTokens()
		if err == nil {
			t.Errorf("Lexer(%q) expected error, got nil", input)
		}
	}
}

func TestLexerUnterminatedBlockString(t *testing.T) {
	_, err := NewLexer(`"""`).ReadAllTokens()
	if err == nil {
		t.Error("expected error for unterminated block string")
	}
}

func TestLexerStringEscapeSequences(t *testing.T) {
	tests := []struct {
		input string
		value string
	}{
		{`"\b"`, "\b"},
		{`"\f"`, "\f"},
		{`"\r"`, "\r"},
	}
	for _, tt := range tests {
		tokens, err := NewLexer(tt.input).ReadAllTokens()
		if err != nil {
			t.Errorf("Lexer(%q) error: %v", tt.input, err)
			continue
		}
		if tokens[0].Value != tt.value {
			t.Errorf("Lexer(%q) value = %q, want %q", tt.input, tokens[0].Value, tt.value)
		}
	}
}

func TestLexerUnterminatedStringEscape(t *testing.T) {
	_, err := NewLexer(`"\`).ReadAllTokens()
	if err == nil {
		t.Error("expected error for unterminated string escape")
	}
}

func TestLexerShortUnicodeEscape(t *testing.T) {
	_, err := NewLexer(`"\u00"`).ReadAllTokens()
	if err == nil {
		t.Error("expected error for short unicode escape")
	}
}

func TestLexerEmptySource(t *testing.T) {
	tokens, err := NewLexer("").ReadAllTokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].Kind != TokenEOF {
		t.Errorf("expected single EOF token, got %d tokens", len(tokens))
	}
}

func TestLexerUnterminatedString(t *testing.T) {
	_, err := NewLexer(`"`).ReadAllTokens()
	if err == nil {
		t.Error("expected error for unterminated string")
	}
}
