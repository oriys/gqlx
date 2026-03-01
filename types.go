package gqlx

import "fmt"

// GraphQLType is the interface implemented by all GraphQL types.
type GraphQLType interface {
	TypeName() string
	String() string
}

// ScalarType represents a GraphQL scalar type.
type ScalarType struct {
	Name_       string
	Description string
	Serialize   func(value interface{}) (interface{}, error)
	ParseValue  func(value interface{}) (interface{}, error)
	ParseLiteral func(value Value) (interface{}, error)
}

func (t *ScalarType) TypeName() string { return t.Name_ }
func (t *ScalarType) String() string   { return t.Name_ }

// ObjectType represents a GraphQL object type.
type ObjectType struct {
	Name_       string
	Description string
	Fields_     FieldMap
	Interfaces_ []*InterfaceType
	IsTypeOf    func(value interface{}) bool
}

func (t *ObjectType) TypeName() string { return t.Name_ }
func (t *ObjectType) String() string   { return t.Name_ }

// FieldMap is a map of field name to field definition.
type FieldMap map[string]*FieldDefinition

// FieldDefinition represents a field definition in an object or interface type.
type FieldDefinition struct {
	Name_             string
	Description       string
	Type              GraphQLType
	Args              ArgumentMap
	Resolve           ResolveFunc
	DeprecationReason string
}

// ArgumentMap is a map of argument name to argument definition.
type ArgumentMap map[string]*ArgumentDefinition

// ArgumentDefinition represents an argument definition.
type ArgumentDefinition struct {
	Name_        string
	Description  string
	Type         GraphQLType
	DefaultValue interface{}
}

// ResolveFunc is a resolver function.
type ResolveFunc func(p ResolveParams) (interface{}, error)

// ResolveParams holds the parameters passed to a resolver.
type ResolveParams struct {
	Source interface{}
	Args   map[string]interface{}
	Info   ResolveInfo
}

// ResolveInfo holds information about the current resolution.
type ResolveInfo struct {
	FieldName  string
	ReturnType GraphQLType
	ParentType *ObjectType
	Path       []interface{}
	Schema     *Schema
	Operation  *OperationDefinition
	Fragments  map[string]*FragmentDefinition
	Variables  map[string]interface{}
}

// InterfaceType represents a GraphQL interface type.
type InterfaceType struct {
	Name_       string
	Description string
	Fields_     FieldMap
	ResolveType func(value interface{}, info ResolveInfo) *ObjectType
}

func (t *InterfaceType) TypeName() string { return t.Name_ }
func (t *InterfaceType) String() string   { return t.Name_ }

// UnionType represents a GraphQL union type.
type UnionType struct {
	Name_       string
	Description string
	Types       []*ObjectType
	ResolveType func(value interface{}, info ResolveInfo) *ObjectType
}

func (t *UnionType) TypeName() string { return t.Name_ }
func (t *UnionType) String() string   { return t.Name_ }

// EnumType represents a GraphQL enum type.
type EnumType struct {
	Name_       string
	Description string
	Values      []*EnumValueDefinition
}

func (t *EnumType) TypeName() string { return t.Name_ }
func (t *EnumType) String() string   { return t.Name_ }

// EnumValueDefinition represents a single value of an enum type.
type EnumValueDefinition struct {
	Name_             string
	Description       string
	Value             interface{}
	DeprecationReason string
}

// InputObjectType represents a GraphQL input object type.
type InputObjectType struct {
	Name_       string
	Description string
	Fields_     InputFieldMap
}

func (t *InputObjectType) TypeName() string { return t.Name_ }
func (t *InputObjectType) String() string   { return t.Name_ }

// InputFieldMap is a map of input field name to input field definition.
type InputFieldMap map[string]*InputFieldDefinition

// InputFieldDefinition represents a field in an input object type.
type InputFieldDefinition struct {
	Name_        string
	Description  string
	Type         GraphQLType
	DefaultValue interface{}
}

// ListOfType represents a GraphQL list type wrapper.
type ListOfType struct {
	OfType GraphQLType
}

func (t *ListOfType) TypeName() string { return "" }
func (t *ListOfType) String() string   { return fmt.Sprintf("[%s]", t.OfType.String()) }

// NonNullOfType represents a GraphQL non-null type wrapper.
type NonNullOfType struct {
	OfType GraphQLType
}

func (t *NonNullOfType) TypeName() string { return "" }
func (t *NonNullOfType) String() string   { return t.OfType.String() + "!" }

// NewList creates a new list type.
func NewList(ofType GraphQLType) *ListOfType {
	return &ListOfType{OfType: ofType}
}

// NewNonNull creates a new non-null type.
func NewNonNull(ofType GraphQLType) *NonNullOfType {
	return &NonNullOfType{OfType: ofType}
}

// IsCompositeType returns true for Object, Interface, and Union types.
func IsCompositeType(t GraphQLType) bool {
	t = UnwrapType(t)
	switch t.(type) {
	case *ObjectType, *InterfaceType, *UnionType:
		return true
	}
	return false
}

// IsInputType returns true if the type can be used as an input type.
func IsInputType(t GraphQLType) bool {
	t = UnwrapType(t)
	switch t.(type) {
	case *ScalarType, *EnumType, *InputObjectType:
		return true
	}
	return false
}

// IsOutputType returns true if the type can be used as an output type.
func IsOutputType(t GraphQLType) bool {
	t = UnwrapType(t)
	switch t.(type) {
	case *ScalarType, *ObjectType, *InterfaceType, *UnionType, *EnumType:
		return true
	}
	return false
}

// IsLeafType returns true for scalar and enum types.
func IsLeafType(t GraphQLType) bool {
	t = UnwrapType(t)
	switch t.(type) {
	case *ScalarType, *EnumType:
		return true
	}
	return false
}

// IsAbstractType returns true for interface and union types.
func IsAbstractType(t GraphQLType) bool {
	t = UnwrapType(t)
	switch t.(type) {
	case *InterfaceType, *UnionType:
		return true
	}
	return false
}

// UnwrapType removes List and NonNull wrappers.
func UnwrapType(t GraphQLType) GraphQLType {
	for {
		switch w := t.(type) {
		case *ListOfType:
			t = w.OfType
		case *NonNullOfType:
			t = w.OfType
		default:
			return t
		}
	}
}

// GetNamedType returns the underlying named type.
func GetNamedType(t GraphQLType) GraphQLType {
	return UnwrapType(t)
}

// GetFields returns the fields of an Object or Interface type.
func GetFields(t GraphQLType) FieldMap {
	switch typ := t.(type) {
	case *ObjectType:
		return typ.Fields_
	case *InterfaceType:
		return typ.Fields_
	}
	return nil
}

// GetPossibleTypes returns the possible concrete types for an abstract type.
func GetPossibleTypes(schema *Schema, t GraphQLType) []*ObjectType {
	switch typ := t.(type) {
	case *ObjectType:
		return []*ObjectType{typ}
	case *InterfaceType:
		return schema.GetImplementations(typ)
	case *UnionType:
		return typ.Types
	}
	return nil
}

// IsNullableType returns true if the type is nullable.
func IsNullableType(t GraphQLType) bool {
	_, isNonNull := t.(*NonNullOfType)
	return !isNonNull
}

// NullableType returns the nullable variant of a type.
func NullableType(t GraphQLType) GraphQLType {
	if nn, ok := t.(*NonNullOfType); ok {
		return nn.OfType
	}
	return t
}
