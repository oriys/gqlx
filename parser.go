package gqlx

import "fmt"

// Parser parses a GraphQL document from a token stream.
type Parser struct {
	lexer *Lexer
	cur   Token
	err   error
}

// Parse parses a GraphQL source string into a Document AST.
func Parse(source string) (*Document, error) {
	p := &Parser{lexer: NewLexer(source)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	return p.parseDocument()
}

func (p *Parser) advance() error {
	tok, err := p.lexer.NextToken()
	if err != nil {
		p.err = err
		return err
	}
	p.cur = tok
	return nil
}

func (p *Parser) expect(kind TokenKind) (Token, error) {
	if p.cur.Kind != kind {
		return Token{}, fmt.Errorf("syntax error at %d:%d: expected %s, got %s", p.cur.Line, p.cur.Col, kind, p.cur.Kind)
	}
	tok := p.cur
	if err := p.advance(); err != nil {
		return Token{}, err
	}
	return tok, nil
}

func (p *Parser) expectKeyword(keyword string) error {
	if p.cur.Kind != TokenName || p.cur.Value != keyword {
		return fmt.Errorf("syntax error at %d:%d: expected %q, got %q", p.cur.Line, p.cur.Col, keyword, p.cur.Value)
	}
	return p.advance()
}

func (p *Parser) peek(kind TokenKind) bool {
	return p.cur.Kind == kind
}

func (p *Parser) skip(kind TokenKind) bool {
	if p.cur.Kind == kind {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) parseDocument() (*Document, error) {
	doc := &Document{}
	for !p.peek(TokenEOF) {
		def, err := p.parseDefinition()
		if err != nil {
			return nil, err
		}
		doc.Definitions = append(doc.Definitions, def)
	}
	if len(doc.Definitions) == 0 {
		return nil, fmt.Errorf("syntax error at %d:%d: expected at least one definition", p.cur.Line, p.cur.Col)
	}
	return doc, nil
}

func (p *Parser) parseDefinition() (Definition, error) {
	if p.peek(TokenBraceL) {
		return p.parseOperationDefinition(OperationQuery)
	}

	if p.cur.Kind == TokenName {
		switch p.cur.Value {
		case "query", "mutation", "subscription":
			return p.parseFullOperationDefinition()
		case "fragment":
			return p.parseFragmentDefinition()
		}
	}

	return nil, fmt.Errorf("syntax error at %d:%d: unexpected %q", p.cur.Line, p.cur.Col, p.cur.Value)
}

func (p *Parser) parseOperationDefinition(op OperationType) (*OperationDefinition, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	selections, err := p.parseSelectionSet()
	if err != nil {
		return nil, err
	}
	return &OperationDefinition{
		Operation:    op,
		SelectionSet: selections,
		Loc:          loc,
	}, nil
}

func (p *Parser) parseFullOperationDefinition() (*OperationDefinition, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	op := operationType(p.cur.Value)
	p.advance()

	var name string
	if p.cur.Kind == TokenName {
		name = p.cur.Value
		p.advance()
	}

	varDefs, err := p.parseVariableDefinitions()
	if err != nil {
		return nil, err
	}

	directives, err := p.parseDirectives(false)
	if err != nil {
		return nil, err
	}

	selections, err := p.parseSelectionSet()
	if err != nil {
		return nil, err
	}

	return &OperationDefinition{
		Operation:           op,
		Name:                name,
		VariableDefinitions: varDefs,
		Directives:          directives,
		SelectionSet:        selections,
		Loc:                 loc,
	}, nil
}

func operationType(s string) OperationType {
	switch s {
	case "mutation":
		return OperationMutation
	case "subscription":
		return OperationSubscription
	default:
		return OperationQuery
	}
}

func (p *Parser) parseFragmentDefinition() (*FragmentDefinition, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	p.expectKeyword("fragment")

	if p.cur.Kind != TokenName || p.cur.Value == "on" {
		return nil, fmt.Errorf("syntax error at %d:%d: expected fragment name", p.cur.Line, p.cur.Col)
	}
	name := p.cur.Value
	p.advance()

	if err := p.expectKeyword("on"); err != nil {
		return nil, err
	}

	typeCond, err := p.expect(TokenName)
	if err != nil {
		return nil, err
	}

	directives, err := p.parseDirectives(false)
	if err != nil {
		return nil, err
	}

	selections, err := p.parseSelectionSet()
	if err != nil {
		return nil, err
	}

	return &FragmentDefinition{
		Name:          name,
		TypeCondition: typeCond.Value,
		Directives:    directives,
		SelectionSet:  selections,
		Loc:           loc,
	}, nil
}

func (p *Parser) parseSelectionSet() ([]Selection, error) {
	if _, err := p.expect(TokenBraceL); err != nil {
		return nil, err
	}

	var selections []Selection
	for !p.peek(TokenBraceR) && !p.peek(TokenEOF) {
		sel, err := p.parseSelection()
		if err != nil {
			return nil, err
		}
		selections = append(selections, sel)
	}

	if _, err := p.expect(TokenBraceR); err != nil {
		return nil, err
	}
	return selections, nil
}

func (p *Parser) parseSelection() (Selection, error) {
	if p.peek(TokenSpread) {
		return p.parseFragment()
	}
	return p.parseField()
}

func (p *Parser) parseField() (*Field, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	nameOrAlias, err := p.expect(TokenName)
	if err != nil {
		return nil, err
	}

	var alias, name string
	if p.skip(TokenColon) {
		alias = nameOrAlias.Value
		nameTok, err := p.expect(TokenName)
		if err != nil {
			return nil, err
		}
		name = nameTok.Value
	} else {
		name = nameOrAlias.Value
	}

	args, err := p.parseArguments(false)
	if err != nil {
		return nil, err
	}

	directives, err := p.parseDirectives(false)
	if err != nil {
		return nil, err
	}

	var selectionSet []Selection
	if p.peek(TokenBraceL) {
		selectionSet, err = p.parseSelectionSet()
		if err != nil {
			return nil, err
		}
	}

	return &Field{
		Alias:        alias,
		Name:         name,
		Arguments:    args,
		Directives:   directives,
		SelectionSet: selectionSet,
		Loc:          loc,
	}, nil
}

func (p *Parser) parseFragment() (Selection, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	p.expect(TokenSpread)

	if p.cur.Kind == TokenName && p.cur.Value != "on" {
		name := p.cur.Value
		p.advance()
		directives, err := p.parseDirectives(false)
		if err != nil {
			return nil, err
		}
		return &FragmentSpread{Name: name, Directives: directives, Loc: loc}, nil
	}

	var typeCond string
	if p.cur.Kind == TokenName && p.cur.Value == "on" {
		p.advance()
		nameTok, err := p.expect(TokenName)
		if err != nil {
			return nil, err
		}
		typeCond = nameTok.Value
	}

	directives, err := p.parseDirectives(false)
	if err != nil {
		return nil, err
	}

	selections, err := p.parseSelectionSet()
	if err != nil {
		return nil, err
	}

	return &InlineFragment{
		TypeCondition: typeCond,
		Directives:    directives,
		SelectionSet:  selections,
		Loc:           loc,
	}, nil
}

func (p *Parser) parseArguments(isConst bool) ([]*Argument, error) {
	if !p.peek(TokenParenL) {
		return nil, nil
	}
	p.advance() // skip (

	var args []*Argument
	for !p.peek(TokenParenR) && !p.peek(TokenEOF) {
		arg, err := p.parseArgument(isConst)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	if _, err := p.expect(TokenParenR); err != nil {
		return nil, err
	}
	return args, nil
}

func (p *Parser) parseArgument(isConst bool) (*Argument, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	name, err := p.expect(TokenName)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenColon); err != nil {
		return nil, err
	}
	val, err := p.parseValue(isConst)
	if err != nil {
		return nil, err
	}
	return &Argument{Name: name.Value, Value: val, Loc: loc}, nil
}

func (p *Parser) parseDirectives(isConst bool) ([]*Directive, error) {
	var directives []*Directive
	for p.peek(TokenAt) {
		d, err := p.parseDirective(isConst)
		if err != nil {
			return nil, err
		}
		directives = append(directives, d)
	}
	return directives, nil
}

func (p *Parser) parseDirective(isConst bool) (*Directive, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	p.expect(TokenAt)
	name, err := p.expect(TokenName)
	if err != nil {
		return nil, err
	}
	args, err := p.parseArguments(isConst)
	if err != nil {
		return nil, err
	}
	return &Directive{Name: name.Value, Arguments: args, Loc: loc}, nil
}

func (p *Parser) parseVariableDefinitions() ([]*VariableDefinition, error) {
	if !p.peek(TokenParenL) {
		return nil, nil
	}
	p.advance() // skip (

	var defs []*VariableDefinition
	for !p.peek(TokenParenR) && !p.peek(TokenEOF) {
		def, err := p.parseVariableDefinition()
		if err != nil {
			return nil, err
		}
		defs = append(defs, def)
	}
	if _, err := p.expect(TokenParenR); err != nil {
		return nil, err
	}
	return defs, nil
}

func (p *Parser) parseVariableDefinition() (*VariableDefinition, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	if _, err := p.expect(TokenDollar); err != nil {
		return nil, err
	}
	name, err := p.expect(TokenName)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenColon); err != nil {
		return nil, err
	}
	typ, err := p.parseTypeReference()
	if err != nil {
		return nil, err
	}

	var defaultVal Value
	if p.skip(TokenEquals) {
		defaultVal, err = p.parseValue(true)
		if err != nil {
			return nil, err
		}
	}

	directives, err := p.parseDirectives(true)
	if err != nil {
		return nil, err
	}

	return &VariableDefinition{
		Variable:     name.Value,
		Type:         typ,
		DefaultValue: defaultVal,
		Directives:   directives,
		Loc:          loc,
	}, nil
}

func (p *Parser) parseTypeReference() (Type, error) {
	var typ Type
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}

	if p.skip(TokenBracketL) {
		inner, err := p.parseTypeReference()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenBracketR); err != nil {
			return nil, err
		}
		typ = &ListType{Type: inner, Loc: loc}
	} else {
		name, err := p.expect(TokenName)
		if err != nil {
			return nil, err
		}
		typ = &NamedType{Name: name.Value, Loc: loc}
	}

	if p.skip(TokenBang) {
		typ = &NonNullType{Type: typ, Loc: loc}
	}
	return typ, nil
}

func (p *Parser) parseValue(isConst bool) (Value, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}

	switch p.cur.Kind {
	case TokenDollar:
		if isConst {
			return nil, fmt.Errorf("syntax error at %d:%d: unexpected variable in constant value", p.cur.Line, p.cur.Col)
		}
		p.advance()
		name, err := p.expect(TokenName)
		if err != nil {
			return nil, err
		}
		return &VariableValue{Name: name.Value, Loc: loc}, nil

	case TokenInt:
		val := p.cur.Value
		p.advance()
		return &IntValue{Value: val, Loc: loc}, nil

	case TokenFloat:
		val := p.cur.Value
		p.advance()
		return &FloatValue{Value: val, Loc: loc}, nil

	case TokenString, TokenBlockString:
		val := p.cur.Value
		p.advance()
		return &StringValue{Value: val, Loc: loc}, nil

	case TokenName:
		val := p.cur.Value
		p.advance()
		switch val {
		case "true":
			return &BooleanValue{Value: true, Loc: loc}, nil
		case "false":
			return &BooleanValue{Value: false, Loc: loc}, nil
		case "null":
			return &NullValue{Loc: loc}, nil
		default:
			return &EnumValue{Value: val, Loc: loc}, nil
		}

	case TokenBracketL:
		return p.parseListValue(isConst)

	case TokenBraceL:
		return p.parseObjectValue(isConst)

	default:
		return nil, fmt.Errorf("syntax error at %d:%d: unexpected token %s", p.cur.Line, p.cur.Col, p.cur.Kind)
	}
}

func (p *Parser) parseListValue(isConst bool) (Value, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	p.advance() // skip [

	var values []Value
	for !p.peek(TokenBracketR) && !p.peek(TokenEOF) {
		val, err := p.parseValue(isConst)
		if err != nil {
			return nil, err
		}
		values = append(values, val)
	}
	if _, err := p.expect(TokenBracketR); err != nil {
		return nil, err
	}
	return &ListValue{Values: values, Loc: loc}, nil
}

func (p *Parser) parseObjectValue(isConst bool) (Value, error) {
	loc := Location{Line: p.cur.Line, Col: p.cur.Col}
	p.advance() // skip {

	var fields []*ObjectField
	for !p.peek(TokenBraceR) && !p.peek(TokenEOF) {
		name, err := p.expect(TokenName)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenColon); err != nil {
			return nil, err
		}
		val, err := p.parseValue(isConst)
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ObjectField{Name: name.Value, Value: val, Loc: loc})
	}
	if _, err := p.expect(TokenBraceR); err != nil {
		return nil, err
	}
	return &ObjectValue{Fields: fields, Loc: loc}, nil
}
