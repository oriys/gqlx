package gqlx

import (
	"testing"
)

func TestTokenKindString(t *testing.T) {
	tests := []struct {
		kind TokenKind
		want string
	}{
		{TokenEOF, "EOF"},
		{TokenBang, "!"},
		{TokenDollar, "$"},
		{TokenAmp, "&"},
		{TokenParenL, "("},
		{TokenParenR, ")"},
		{TokenSpread, "..."},
		{TokenColon, ":"},
		{TokenEquals, "="},
		{TokenAt, "@"},
		{TokenBracketL, "["},
		{TokenBracketR, "]"},
		{TokenBraceL, "{"},
		{TokenPipe, "|"},
		{TokenBraceR, "}"},
		{TokenName, "Name"},
		{TokenInt, "Int"},
		{TokenFloat, "Float"},
		{TokenString, "String"},
		{TokenBlockString, "BlockString"},
		{TokenKind(999), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("TokenKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
