package gqlx

import (
	"testing"
)

func TestNewSchema(t *testing.T) {
	queryType := &ObjectType{
		Name_: "Query",
		Fields_: FieldMap{
			"hello": {Type: StringScalar},
		},
	}

	schema, err := NewSchema(SchemaConfig{Query: queryType})
	if err != nil {
		t.Fatal(err)
	}

	if schema.QueryType != queryType {
		t.Error("QueryType mismatch")
	}
	if schema.MutationType != nil {
		t.Error("MutationType should be nil")
	}
	if schema.Type("Query") != queryType {
		t.Error("Type('Query') should return query type")
	}
	if schema.Type("String") != StringScalar {
		t.Error("Built-in String scalar should be available")
	}
}

func TestSchemaRequiresQuery(t *testing.T) {
	_, err := NewSchema(SchemaConfig{})
	if err == nil {
		t.Error("expected error for missing Query type")
	}
}

func TestSchemaWithMutation(t *testing.T) {
	mutationType := &ObjectType{
		Name_: "Mutation",
		Fields_: FieldMap{
			"addItem": {Type: StringScalar},
		},
	}
	schema, err := NewSchema(SchemaConfig{
		Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
		Mutation: mutationType,
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema.MutationType != mutationType {
		t.Error("MutationType mismatch")
	}
}

func TestSchemaTypeMap(t *testing.T) {
	userType := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
			"id":   {Type: IDScalar},
		},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"user": {Type: userType},
			},
		},
	})

	typeMap := schema.TypeMap()
	if _, ok := typeMap["User"]; !ok {
		t.Error("User type not in type map")
	}
	if _, ok := typeMap["Int"]; !ok {
		t.Error("Int scalar not in type map")
	}
}

func TestSchemaInterfaces(t *testing.T) {
	nodeIface := &InterfaceType{
		Name_: "Node",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
		},
	}
	userType := &ObjectType{
		Name_:       "User",
		Interfaces_: []*InterfaceType{nodeIface},
		Fields_: FieldMap{
			"id":   {Type: NewNonNull(IDScalar)},
			"name": {Type: StringScalar},
		},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"node": {Type: nodeIface},
			},
		},
		Types: []GraphQLType{userType},
	})

	impls := schema.GetImplementations(nodeIface)
	if len(impls) != 1 || impls[0] != userType {
		t.Errorf("expected User to implement Node, got %v", impls)
	}

	if !schema.IsPossibleType(nodeIface, userType) {
		t.Error("User should be possible type for Node")
	}
}

func TestSchemaUnionPossibleType(t *testing.T) {
	human := &ObjectType{Name_: "Human", Fields_: FieldMap{"name": {Type: StringScalar}}}
	droid := &ObjectType{Name_: "Droid", Fields_: FieldMap{"name": {Type: StringScalar}}}
	union := &UnionType{Name_: "Character", Types: []*ObjectType{human, droid}}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"character": {Type: union},
			},
		},
	})

	if !schema.IsPossibleType(union, human) {
		t.Error("Human should be possible type for Character union")
	}
	if !schema.IsPossibleType(union, droid) {
		t.Error("Droid should be possible type for Character union")
	}

	other := &ObjectType{Name_: "Other"}
	if schema.IsPossibleType(union, other) {
		t.Error("Other should not be possible type for Character union")
	}
}

func TestSchemaDirectives(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
	})

	dirs := schema.Directives()
	if len(dirs) != 4 {
		t.Fatalf("expected 4 directives, got %d", len(dirs))
	}

	if schema.Directive("skip") == nil {
		t.Error("skip directive not found")
	}
	if schema.Directive("nonexistent") != nil {
		t.Error("nonexistent directive should be nil")
	}
}

func TestSchemaCustomDirectives(t *testing.T) {
	customDir := &DirectiveDefinition{Name_: "custom", Locations: []DirectiveLocation{DirectiveLocationField}}
	schema, _ := NewSchema(SchemaConfig{
		Query:      &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
		Directives: []*DirectiveDefinition{customDir},
	})

	if len(schema.Directives()) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(schema.Directives()))
	}
	if schema.Directive("custom") != customDir {
		t.Error("custom directive not found")
	}
}

func TestSchemaWithSubscription(t *testing.T) {
	subType := &ObjectType{
		Name_: "Subscription",
		Fields_: FieldMap{
			"messageAdded": {Type: StringScalar},
		},
	}
	schema, err := NewSchema(SchemaConfig{
		Query:        &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
		Subscription: subType,
	})
	if err != nil {
		t.Fatal(err)
	}
	if schema.SubscriptionType != subType {
		t.Error("SubscriptionType mismatch")
	}
}

func TestSchemaWithEnumType(t *testing.T) {
	colorEnum := &EnumType{
		Name_: "Color",
		Values: []*EnumValueDefinition{
			{Name_: "RED"},
			{Name_: "GREEN"},
			{Name_: "BLUE"},
		},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"color": {Type: colorEnum},
			},
		},
	})

	if schema.Type("Color") != colorEnum {
		t.Error("Color enum not in type map")
	}
}

func TestSchemaWithInputType(t *testing.T) {
	input := &InputObjectType{
		Name_: "UserInput",
		Fields_: InputFieldMap{
			"name": {Type: StringScalar},
		},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"user": {
					Type: StringScalar,
					Args: ArgumentMap{
						"input": {Type: input},
					},
				},
			},
		},
	})

	if schema.Type("UserInput") != input {
		t.Error("UserInput not in type map")
	}
}

func TestSchemaWithListAndNonNull(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"names": {Type: NewNonNull(NewList(NewNonNull(StringScalar)))},
			},
		},
	})

	if schema.Type("String") != StringScalar {
		t.Error("String type should be collected through wrappers")
	}
}
