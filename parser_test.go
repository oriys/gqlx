package gqlx

import (
	"testing"
)

func TestParseSimpleQuery(t *testing.T) {
	doc, err := Parse(`{ hero { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Definitions) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(doc.Definitions))
	}
	op, ok := doc.Definitions[0].(*OperationDefinition)
	if !ok {
		t.Fatal("expected OperationDefinition")
	}
	if op.Operation != OperationQuery {
		t.Errorf("operation = %v, want Query", op.Operation)
	}
	if len(op.SelectionSet) != 1 {
		t.Fatalf("expected 1 selection, got %d", len(op.SelectionSet))
	}
	field, ok := op.SelectionSet[0].(*Field)
	if !ok {
		t.Fatal("expected Field")
	}
	if field.Name != "hero" {
		t.Errorf("field name = %q, want \"hero\"", field.Name)
	}
}

func TestParseNamedQuery(t *testing.T) {
	doc, err := Parse(`query GetHero { hero { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if op.Name != "GetHero" {
		t.Errorf("name = %q, want \"GetHero\"", op.Name)
	}
}

func TestParseMutation(t *testing.T) {
	doc, err := Parse(`mutation { addHero(name: "Luke") { id } }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if op.Operation != OperationMutation {
		t.Errorf("operation = %v, want Mutation", op.Operation)
	}
}

func TestParseSubscription(t *testing.T) {
	doc, err := Parse(`subscription { heroAdded { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if op.Operation != OperationSubscription {
		t.Errorf("operation = %v, want Subscription", op.Operation)
	}
}

func TestParseFieldAlias(t *testing.T) {
	doc, err := Parse(`{ smallHero: hero(size: "small") { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	if field.Alias != "smallHero" {
		t.Errorf("alias = %q, want \"smallHero\"", field.Alias)
	}
	if field.Name != "hero" {
		t.Errorf("name = %q, want \"hero\"", field.Name)
	}
}

func TestParseArguments(t *testing.T) {
	doc, err := Parse(`{ hero(id: 1, name: "Luke") { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	if len(field.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(field.Arguments))
	}
	if field.Arguments[0].Name != "id" {
		t.Errorf("arg[0] name = %q, want \"id\"", field.Arguments[0].Name)
	}
}

func TestParseVariables(t *testing.T) {
	doc, err := Parse(`query GetHero($id: ID!, $name: String = "default") { hero(id: $id) { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if len(op.VariableDefinitions) != 2 {
		t.Fatalf("expected 2 variable definitions, got %d", len(op.VariableDefinitions))
	}
	if op.VariableDefinitions[0].Variable != "id" {
		t.Errorf("var[0] = %q, want \"id\"", op.VariableDefinitions[0].Variable)
	}
	if _, ok := op.VariableDefinitions[0].Type.(*NonNullType); !ok {
		t.Error("var[0] type should be NonNull")
	}
	if op.VariableDefinitions[1].DefaultValue == nil {
		t.Error("var[1] should have default value")
	}
}

func TestParseFragment(t *testing.T) {
	doc, err := Parse(`
		query { hero { ...heroFields } }
		fragment heroFields on Hero { name age }
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(doc.Definitions))
	}
	frag, ok := doc.Definitions[1].(*FragmentDefinition)
	if !ok {
		t.Fatal("expected FragmentDefinition")
	}
	if frag.Name != "heroFields" {
		t.Errorf("fragment name = %q, want \"heroFields\"", frag.Name)
	}
	if frag.TypeCondition != "Hero" {
		t.Errorf("type condition = %q, want \"Hero\"", frag.TypeCondition)
	}
}

func TestParseInlineFragment(t *testing.T) {
	doc, err := Parse(`{ hero { ... on Human { height } } }`)
	if err != nil {
		t.Fatal(err)
	}
	heroField := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	inline, ok := heroField.SelectionSet[0].(*InlineFragment)
	if !ok {
		t.Fatal("expected InlineFragment")
	}
	if inline.TypeCondition != "Human" {
		t.Errorf("type condition = %q, want \"Human\"", inline.TypeCondition)
	}
}

func TestParseInlineFragmentWithoutTypeCondition(t *testing.T) {
	doc, err := Parse(`{ hero { ... { name } } }`)
	if err != nil {
		t.Fatal(err)
	}
	heroField := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	inline, ok := heroField.SelectionSet[0].(*InlineFragment)
	if !ok {
		t.Fatal("expected InlineFragment")
	}
	if inline.TypeCondition != "" {
		t.Errorf("type condition = %q, want empty", inline.TypeCondition)
	}
}

func TestParseDirectives(t *testing.T) {
	doc, err := Parse(`{ hero @skip(if: true) { name @include(if: false) } }`)
	if err != nil {
		t.Fatal(err)
	}
	heroField := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	if len(heroField.Directives) != 1 || heroField.Directives[0].Name != "skip" {
		t.Error("expected @skip directive on hero")
	}
	nameField := heroField.SelectionSet[0].(*Field)
	if len(nameField.Directives) != 1 || nameField.Directives[0].Name != "include" {
		t.Error("expected @include directive on name")
	}
}

func TestParseAllValueTypes(t *testing.T) {
	doc, err := Parse(`{
		field(
			intArg: 42
			floatArg: 3.14
			stringArg: "hello"
			boolArg: true
			nullArg: null
			enumArg: ACTIVE
			listArg: [1, 2, 3]
			objectArg: {key: "value", nested: {a: 1}}
		)
	}`)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	if len(field.Arguments) != 8 {
		t.Fatalf("expected 8 arguments, got %d", len(field.Arguments))
	}

	// Check types
	if _, ok := field.Arguments[0].Value.(*IntValue); !ok {
		t.Error("arg[0] should be IntValue")
	}
	if _, ok := field.Arguments[1].Value.(*FloatValue); !ok {
		t.Error("arg[1] should be FloatValue")
	}
	if _, ok := field.Arguments[2].Value.(*StringValue); !ok {
		t.Error("arg[2] should be StringValue")
	}
	if _, ok := field.Arguments[3].Value.(*BooleanValue); !ok {
		t.Error("arg[3] should be BooleanValue")
	}
	if _, ok := field.Arguments[4].Value.(*NullValue); !ok {
		t.Error("arg[4] should be NullValue")
	}
	if _, ok := field.Arguments[5].Value.(*EnumValue); !ok {
		t.Error("arg[5] should be EnumValue")
	}
	if _, ok := field.Arguments[6].Value.(*ListValue); !ok {
		t.Error("arg[6] should be ListValue")
	}
	if _, ok := field.Arguments[7].Value.(*ObjectValue); !ok {
		t.Error("arg[7] should be ObjectValue")
	}
}

func TestParseVariableInValue(t *testing.T) {
	doc, err := Parse(`query($id: ID!) { hero(id: $id) { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	varVal, ok := field.Arguments[0].Value.(*VariableValue)
	if !ok {
		t.Fatal("expected VariableValue")
	}
	if varVal.Name != "id" {
		t.Errorf("variable name = %q, want \"id\"", varVal.Name)
	}
}

func TestParseListType(t *testing.T) {
	doc, err := Parse(`query($ids: [ID!]!) { heroes(ids: $ids) { name } }`)
	if err != nil {
		t.Fatal(err)
	}
	varDef := doc.Definitions[0].(*OperationDefinition).VariableDefinitions[0]
	nn, ok := varDef.Type.(*NonNullType)
	if !ok {
		t.Fatal("expected NonNullType")
	}
	list, ok := nn.Type.(*ListType)
	if !ok {
		t.Fatal("expected ListType")
	}
	innerNN, ok := list.Type.(*NonNullType)
	if !ok {
		t.Fatal("expected inner NonNullType")
	}
	named, ok := innerNN.Type.(*NamedType)
	if !ok {
		t.Fatal("expected NamedType")
	}
	if named.Name != "ID" {
		t.Errorf("type name = %q, want \"ID\"", named.Name)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []string{
		"",                    // empty document
		"unknown_keyword {}",  // unexpected keyword
		"{ field(",           // unterminated arguments
		"fragment on on T {}", // "on" cannot be fragment name
	}
	for _, input := range tests {
		_, err := Parse(input)
		if err == nil {
			t.Errorf("Parse(%q) expected error", input)
		}
	}
}

func TestParseVariableInConstContext(t *testing.T) {
	_, err := Parse(`query($x: Int = $y) { field }`)
	if err == nil {
		t.Error("expected error for variable in const context")
	}
}

func TestParseMultipleOperations(t *testing.T) {
	doc, err := Parse(`
		query GetA { a }
		query GetB { b }
	`)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(doc.Definitions))
	}
}

func TestParseOperationTypeString(t *testing.T) {
	if OperationQuery.String() != "query" {
		t.Errorf("OperationQuery.String() = %q", OperationQuery.String())
	}
	if OperationMutation.String() != "mutation" {
		t.Errorf("OperationMutation.String() = %q", OperationMutation.String())
	}
	if OperationSubscription.String() != "subscription" {
		t.Errorf("OperationSubscription.String() = %q", OperationSubscription.String())
	}
	if OperationType(99).String() != "unknown" {
		t.Errorf("Unknown operation type string = %q", OperationType(99).String())
	}
}

func TestParseFragmentSpreadDirectives(t *testing.T) {
	doc, err := Parse(`
		{ hero { ...heroFields @skip(if: true) } }
		fragment heroFields on Hero { name }
	`)
	if err != nil {
		t.Fatal(err)
	}
	hero := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	spread, ok := hero.SelectionSet[0].(*FragmentSpread)
	if !ok {
		t.Fatal("expected FragmentSpread")
	}
	if len(spread.Directives) != 1 {
		t.Errorf("expected 1 directive, got %d", len(spread.Directives))
	}
}

func TestASTTypeString(t *testing.T) {
	named := &NamedType{Name: "String"}
	if named.String() != "String" {
		t.Errorf("NamedType.String() = %q", named.String())
	}

	list := &ListType{Type: named}
	if list.String() != "[String]" {
		t.Errorf("ListType.String() = %q", list.String())
	}

	nn := &NonNullType{Type: named}
	if nn.String() != "String!" {
		t.Errorf("NonNullType.String() = %q", nn.String())
	}
}

func TestParseOperationDirectives(t *testing.T) {
	doc, err := Parse(`query @skip(if: true) { field }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if len(op.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(op.Directives))
	}
}

func TestParseVariableDefinitionDirectives(t *testing.T) {
	doc, err := Parse(`query($x: Int @deprecated) { field(a: $x) }`)
	if err != nil {
		t.Fatal(err)
	}
	op := doc.Definitions[0].(*OperationDefinition)
	if len(op.VariableDefinitions[0].Directives) != 1 {
		t.Fatal("expected directive on variable definition")
	}
}

func TestParseFragmentDefinitionDirectives(t *testing.T) {
	doc, err := Parse(`
		{ ...f }
		fragment f on Query @deprecated { field }
	`)
	if err != nil {
		t.Fatal(err)
	}
	frag := doc.Definitions[1].(*FragmentDefinition)
	if len(frag.Directives) != 1 {
		t.Fatal("expected directive on fragment definition")
	}
}

func TestParseBoolFalse(t *testing.T) {
	doc, err := Parse(`{ field(arg: false) }`)
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	bv, ok := field.Arguments[0].Value.(*BooleanValue)
	if !ok {
		t.Fatal("expected BooleanValue")
	}
	if bv.Value != false {
		t.Error("expected false")
	}
}

func TestParseBlockStringValue(t *testing.T) {
	doc, err := Parse("{ field(arg: \"\"\"hello\"\"\") }")
	if err != nil {
		t.Fatal(err)
	}
	field := doc.Definitions[0].(*OperationDefinition).SelectionSet[0].(*Field)
	sv, ok := field.Arguments[0].Value.(*StringValue)
	if !ok {
		t.Fatal("expected StringValue")
	}
	if sv.Value != "hello" {
		t.Errorf("value = %q, want \"hello\"", sv.Value)
	}
}
