package gqlx

import (
	"testing"
)

func TestTypeHelpers(t *testing.T) {
	obj := &ObjectType{Name_: "User", Fields_: FieldMap{}}
	iface := &InterfaceType{Name_: "Node", Fields_: FieldMap{}}
	union := &UnionType{Name_: "SearchResult", Types: []*ObjectType{obj}}
	enum := &EnumType{Name_: "Color"}
	scalar := &ScalarType{Name_: "CustomScalar"}
	input := &InputObjectType{Name_: "UserInput"}

	// TypeName
	if obj.TypeName() != "User" {
		t.Errorf("obj.TypeName() = %q", obj.TypeName())
	}
	if iface.TypeName() != "Node" {
		t.Errorf("iface.TypeName() = %q", iface.TypeName())
	}
	if union.TypeName() != "SearchResult" {
		t.Errorf("union.TypeName() = %q", union.TypeName())
	}
	if enum.TypeName() != "Color" {
		t.Errorf("enum.TypeName() = %q", enum.TypeName())
	}
	if scalar.TypeName() != "CustomScalar" {
		t.Errorf("scalar.TypeName() = %q", scalar.TypeName())
	}
	if input.TypeName() != "UserInput" {
		t.Errorf("input.TypeName() = %q", input.TypeName())
	}

	// String
	list := NewList(obj)
	nn := NewNonNull(obj)
	if list.String() != "[User]" {
		t.Errorf("list.String() = %q", list.String())
	}
	if nn.String() != "User!" {
		t.Errorf("nn.String() = %q", nn.String())
	}
	if list.TypeName() != "" {
		t.Errorf("list.TypeName() should be empty")
	}
	if nn.TypeName() != "" {
		t.Errorf("nn.TypeName() should be empty")
	}

	// IsCompositeType
	if !IsCompositeType(obj) {
		t.Error("Object should be composite")
	}
	if !IsCompositeType(iface) {
		t.Error("Interface should be composite")
	}
	if !IsCompositeType(union) {
		t.Error("Union should be composite")
	}
	if IsCompositeType(scalar) {
		t.Error("Scalar should not be composite")
	}
	if IsCompositeType(NewNonNull(scalar)) {
		t.Error("NonNull(Scalar) should not be composite")
	}

	// IsInputType
	if !IsInputType(scalar) {
		t.Error("Scalar should be input type")
	}
	if !IsInputType(enum) {
		t.Error("Enum should be input type")
	}
	if !IsInputType(input) {
		t.Error("InputObject should be input type")
	}
	if IsInputType(obj) {
		t.Error("Object should not be input type")
	}

	// IsOutputType
	if !IsOutputType(obj) {
		t.Error("Object should be output type")
	}
	if !IsOutputType(iface) {
		t.Error("Interface should be output type")
	}
	if !IsOutputType(union) {
		t.Error("Union should be output type")
	}
	if !IsOutputType(enum) {
		t.Error("Enum should be output type")
	}
	if IsOutputType(input) {
		t.Error("InputObject should not be output type")
	}

	// IsLeafType
	if !IsLeafType(scalar) {
		t.Error("Scalar should be leaf type")
	}
	if !IsLeafType(enum) {
		t.Error("Enum should be leaf type")
	}
	if IsLeafType(obj) {
		t.Error("Object should not be leaf type")
	}

	// IsAbstractType
	if !IsAbstractType(iface) {
		t.Error("Interface should be abstract")
	}
	if !IsAbstractType(union) {
		t.Error("Union should be abstract")
	}
	if IsAbstractType(obj) {
		t.Error("Object should not be abstract")
	}

	// IsNullableType
	if !IsNullableType(obj) {
		t.Error("Object should be nullable")
	}
	if IsNullableType(nn) {
		t.Error("NonNull should not be nullable")
	}

	// NullableType
	if NullableType(nn) != obj {
		t.Error("NullableType(NonNull(obj)) should return obj")
	}
	if NullableType(obj) != obj {
		t.Error("NullableType(obj) should return obj")
	}

	// UnwrapType
	if UnwrapType(NewNonNull(NewList(obj))) != obj {
		t.Error("UnwrapType should return underlying named type")
	}

	// GetNamedType
	if GetNamedType(NewNonNull(obj)) != obj {
		t.Error("GetNamedType should return underlying type")
	}

	// GetFields
	if GetFields(obj) == nil {
		t.Error("GetFields(Object) should return fields")
	}
	if GetFields(iface) == nil {
		t.Error("GetFields(Interface) should return fields")
	}
	if GetFields(scalar) != nil {
		t.Error("GetFields(Scalar) should return nil")
	}
}

func TestGetPossibleTypes(t *testing.T) {
	human := &ObjectType{Name_: "Human"}
	droid := &ObjectType{Name_: "Droid"}

	iface := &InterfaceType{Name_: "Character"}
	human.Interfaces_ = []*InterfaceType{iface}
	droid.Interfaces_ = []*InterfaceType{iface}

	union := &UnionType{Name_: "SearchResult", Types: []*ObjectType{human, droid}}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"hero": {Type: iface},
				"search": {Type: union},
			},
		},
		Types: []GraphQLType{human, droid},
	})

	if pts := GetPossibleTypes(schema, iface); len(pts) != 2 {
		t.Errorf("interface possible types = %d, want 2", len(pts))
	}
	if pts := GetPossibleTypes(schema, union); len(pts) != 2 {
		t.Errorf("union possible types = %d, want 2", len(pts))
	}
	if pts := GetPossibleTypes(schema, human); len(pts) != 1 {
		t.Errorf("object possible types = %d, want 1", len(pts))
	}
}
