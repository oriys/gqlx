package gqlx

// Location represents a position in a source document.
type Location struct {
	Line int
	Col  int
}

// Document is the root AST node.
type Document struct {
	Definitions []Definition
}

// Definition is a top-level definition in a document.
type Definition interface {
	definitionNode()
}

// OperationDefinition represents a query, mutation, or subscription operation.
type OperationDefinition struct {
	Operation           OperationType
	Name                string
	VariableDefinitions []*VariableDefinition
	Directives          []*Directive
	SelectionSet        []Selection
	Loc                 Location
}

func (*OperationDefinition) definitionNode() {}

// OperationType identifies the type of operation.
type OperationType int

const (
	OperationQuery OperationType = iota
	OperationMutation
	OperationSubscription
)

func (o OperationType) String() string {
	switch o {
	case OperationQuery:
		return "query"
	case OperationMutation:
		return "mutation"
	case OperationSubscription:
		return "subscription"
	}
	return "unknown"
}

// FragmentDefinition represents a named fragment.
type FragmentDefinition struct {
	Name          string
	TypeCondition string
	Directives    []*Directive
	SelectionSet  []Selection
	Loc           Location
}

func (*FragmentDefinition) definitionNode() {}

// Selection is a selection in a selection set.
type Selection interface {
	selectionNode()
}

// Field represents a field selection.
type Field struct {
	Alias        string
	Name         string
	Arguments    []*Argument
	Directives   []*Directive
	SelectionSet []Selection
	Loc          Location
}

func (*Field) selectionNode() {}

// FragmentSpread represents a fragment spread (...FragmentName).
type FragmentSpread struct {
	Name       string
	Directives []*Directive
	Loc        Location
}

func (*FragmentSpread) selectionNode() {}

// InlineFragment represents an inline fragment (... on Type { ... }).
type InlineFragment struct {
	TypeCondition string
	Directives    []*Directive
	SelectionSet  []Selection
	Loc           Location
}

func (*InlineFragment) selectionNode() {}

// Argument represents a field or directive argument.
type Argument struct {
	Name  string
	Value Value
	Loc   Location
}

// Directive represents a directive usage.
type Directive struct {
	Name      string
	Arguments []*Argument
	Loc       Location
}

// VariableDefinition represents a variable definition in an operation.
type VariableDefinition struct {
	Variable     string
	Type         Type
	DefaultValue Value
	Directives   []*Directive
	Loc          Location
}

// Type is a reference to a GraphQL type in the AST.
type Type interface {
	typeNode()
	String() string
}

// NamedType is a reference to a named type.
type NamedType struct {
	Name string
	Loc  Location
}

func (*NamedType) typeNode() {}
func (t *NamedType) String() string {
	return t.Name
}

// ListType is a list type wrapper.
type ListType struct {
	Type Type
	Loc  Location
}

func (*ListType) typeNode() {}
func (t *ListType) String() string {
	return "[" + t.Type.String() + "]"
}

// NonNullType is a non-null type wrapper.
type NonNullType struct {
	Type Type // NamedType or ListType
	Loc  Location
}

func (*NonNullType) typeNode() {}
func (t *NonNullType) String() string {
	return t.Type.String() + "!"
}

// Value is an AST value node.
type Value interface {
	valueNode()
}

// VariableValue represents a variable reference.
type VariableValue struct {
	Name string
	Loc  Location
}

func (*VariableValue) valueNode() {}

// IntValue represents an integer literal.
type IntValue struct {
	Value string
	Loc   Location
}

func (*IntValue) valueNode() {}

// FloatValue represents a float literal.
type FloatValue struct {
	Value string
	Loc   Location
}

func (*FloatValue) valueNode() {}

// StringValue represents a string literal.
type StringValue struct {
	Value string
	Loc   Location
}

func (*StringValue) valueNode() {}

// BooleanValue represents a boolean literal.
type BooleanValue struct {
	Value bool
	Loc   Location
}

func (*BooleanValue) valueNode() {}

// NullValue represents a null literal.
type NullValue struct {
	Loc Location
}

func (*NullValue) valueNode() {}

// EnumValue represents an enum value literal.
type EnumValue struct {
	Value string
	Loc   Location
}

func (*EnumValue) valueNode() {}

// ListValue represents a list literal.
type ListValue struct {
	Values []Value
	Loc    Location
}

func (*ListValue) valueNode() {}

// ObjectValue represents an object literal.
type ObjectValue struct {
	Fields []*ObjectField
	Loc    Location
}

func (*ObjectValue) valueNode() {}

// ObjectField represents a field in an object literal.
type ObjectField struct {
	Name  string
	Value Value
	Loc   Location
}
