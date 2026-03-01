package gqlx

// Token represents a lexical token of the GraphQL language.
type Token struct {
	Kind  TokenKind
	Value string
	Line  int
	Col   int
}

// TokenKind identifies the type of a lexical token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenBang
	TokenDollar
	TokenAmp
	TokenParenL
	TokenParenR
	TokenSpread
	TokenColon
	TokenEquals
	TokenAt
	TokenBracketL
	TokenBracketR
	TokenBraceL
	TokenPipe
	TokenBraceR
	TokenName
	TokenInt
	TokenFloat
	TokenString
	TokenBlockString
)

var tokenKindNames = map[TokenKind]string{
	TokenEOF:         "EOF",
	TokenBang:        "!",
	TokenDollar:      "$",
	TokenAmp:         "&",
	TokenParenL:      "(",
	TokenParenR:      ")",
	TokenSpread:      "...",
	TokenColon:       ":",
	TokenEquals:      "=",
	TokenAt:          "@",
	TokenBracketL:    "[",
	TokenBracketR:    "]",
	TokenBraceL:      "{",
	TokenPipe:        "|",
	TokenBraceR:      "}",
	TokenName:        "Name",
	TokenInt:         "Int",
	TokenFloat:       "Float",
	TokenString:      "String",
	TokenBlockString: "BlockString",
}

func (k TokenKind) String() string {
	if s, ok := tokenKindNames[k]; ok {
		return s
	}
	return "Unknown"
}
