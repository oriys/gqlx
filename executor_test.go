package gqlx

import (
	"fmt"
	"testing"
)

func heroSchema() *Schema {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name":    {Type: StringScalar},
			"age":     {Type: IntScalar},
			"friends": nil, // set below
		},
	}

	humanType.Fields_["friends"] = &FieldDefinition{Type: NewList(humanType)}

	colorEnum := &EnumType{
		Name_: "Color",
		Values: []*EnumValueDefinition{
			{Name_: "RED", Value: "red"},
			{Name_: "GREEN", Value: "green"},
		},
	}

	queryType := &ObjectType{
		Name_: "Query",
		Fields_: FieldMap{
			"hero": {
				Type: humanType,
				Args: ArgumentMap{
					"id": {Name_: "id", Type: IDScalar},
				},
				Resolve: func(p ResolveParams) (interface{}, error) {
					return map[string]interface{}{
						"name":    "Luke",
						"age":     25,
						"friends": []map[string]interface{}{{"name": "Han", "age": 30}},
					}, nil
				},
			},
			"heroes": {
				Type: NewList(humanType),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return []map[string]interface{}{
						{"name": "Luke", "age": 25},
						{"name": "Leia", "age": 25},
					}, nil
				},
			},
			"hello": {
				Type: StringScalar,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return "world", nil
				},
			},
			"color": {
				Type: colorEnum,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return "red", nil
				},
			},
			"error": {
				Type: StringScalar,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return nil, fmt.Errorf("something went wrong")
				},
			},
			"errorNonNull": {
				Type: NewNonNull(StringScalar),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return nil, fmt.Errorf("non-null error")
				},
			},
			"nullableHero": {
				Type: humanType,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return nil, nil
				},
			},
		},
	}

	droidType := &ObjectType{
		Name_: "Droid",
		Fields_: FieldMap{
			"name":      {Type: StringScalar},
			"primaryFn": {Type: StringScalar},
		},
	}

	mutationType := &ObjectType{
		Name_: "Mutation",
		Fields_: FieldMap{
			"addHero": {
				Type: humanType,
				Args: ArgumentMap{
					"name": {Name_: "name", Type: NewNonNull(StringScalar)},
				},
				Resolve: func(p ResolveParams) (interface{}, error) {
					return map[string]interface{}{
						"name": p.Args["name"],
						"age":  0,
					}, nil
				},
			},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
		Types:    []GraphQLType{droidType},
	})
	return schema
}

func TestExecuteSimpleQuery(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hello }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["hello"] != "world" {
		t.Errorf("hello = %v", data["hello"])
	}
}

func TestExecuteNestedQuery(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { name age } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
	if hero["age"] != 25 {
		t.Errorf("age = %v", hero["age"])
	}
}

func TestExecuteListQuery(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ heroes { name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	heroes := data["heroes"].([]interface{})
	if len(heroes) != 2 {
		t.Fatalf("expected 2 heroes, got %d", len(heroes))
	}
}

func TestExecuteMutation(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `mutation { addHero(name: "Yoda") { name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["addHero"].(map[string]interface{})
	if hero["name"] != "Yoda" {
		t.Errorf("name = %v", hero["name"])
	}
}

func TestExecuteWithVariables(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `query($id: ID) { hero(id: $id) { name } }`, map[string]interface{}{"id": "1"}, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
}

func TestExecuteWithAlias(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ h: hello }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["h"] != "world" {
		t.Errorf("h = %v", data["h"])
	}
}

func TestExecuteTypename(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { __typename name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["__typename"] != "Human" {
		t.Errorf("__typename = %v", hero["__typename"])
	}
}

func TestExecuteSkipDirective(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hello @skip(if: true) }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if _, ok := data["hello"]; ok {
		t.Error("hello should be skipped")
	}
}

func TestExecuteIncludeDirective(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hello @include(if: false) }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if _, ok := data["hello"]; ok {
		t.Error("hello should not be included")
	}
}

func TestExecuteIncludeTrue(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hello @include(if: true) }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["hello"] != "world" {
		t.Errorf("hello = %v", data["hello"])
	}
}

func TestExecuteResolverError(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ error }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected errors")
	}
	if result.Errors[0].Message != "something went wrong" {
		t.Errorf("error message = %q", result.Errors[0].Message)
	}
}

func TestExecuteNonNullResolverError(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ errorNonNull }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected errors")
	}
}

func TestExecuteNullableHero(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ nullableHero { name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["nullableHero"] != nil {
		t.Errorf("nullableHero = %v", data["nullableHero"])
	}
}

func TestExecuteEnumField(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ color }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["color"] != "RED" {
		t.Errorf("color = %v", data["color"])
	}
}

func TestExecuteNestedList(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { friends { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	friends := hero["friends"].([]interface{})
	if len(friends) != 1 {
		t.Fatalf("expected 1 friend, got %d", len(friends))
	}
	friend := friends[0].(map[string]interface{})
	if friend["name"] != "Han" {
		t.Errorf("friend name = %v", friend["name"])
	}
}

func TestExecuteIntrospectionSchema(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __schema { queryType { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	qt := schemaObj["queryType"].(map[string]interface{})
	if qt["name"] != "Query" {
		t.Errorf("queryType name = %v", qt["name"])
	}
}

func TestExecuteIntrospectionType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __type(name: "Human") { name kind fields { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["name"] != "Human" {
		t.Errorf("name = %v", typeObj["name"])
	}
	if typeObj["kind"] != "OBJECT" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionTypeUnknown(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __type(name: "Unknown") { name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["__type"] != nil {
		t.Errorf("__type = %v, want nil", data["__type"])
	}
}

func TestExecuteMultipleOperations(t *testing.T) {
	schema := heroSchema()

	// Must provide operation name
	result := Do(schema, `query A { hello } query B { hero { name } }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected error for ambiguous operation")
	}

	// Named operation
	result = Do(schema, `query A { hello } query B { hero { name } }`, nil, "A")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	if data["hello"] != "world" {
		t.Errorf("hello = %v", data["hello"])
	}
}

func TestExecuteUnknownOperation(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `query A { hello }`, nil, "B")
	if !result.HasErrors() {
		t.Error("expected error for unknown operation")
	}
}

func TestExecuteNoOperation(t *testing.T) {
	schema := heroSchema()
	doc := &Document{}
	result := Execute(ExecuteParams{Schema: schema, Document: doc})
	if !result.HasErrors() {
		t.Error("expected error for no operation")
	}
}

func TestExecuteMissingSchema(t *testing.T) {
	result := Execute(ExecuteParams{Document: &Document{}})
	if !result.HasErrors() {
		t.Error("expected error for missing schema")
	}
}

func TestDoParseError(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ invalid syntax !!!`, nil, "")
	if !result.HasErrors() {
		t.Error("expected parse error")
	}
}

func TestDoValidationError(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ unknownField }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected validation error")
	}
}

func TestExecuteWithNoResolverUsesDefaultResolution(t *testing.T) {
	type User struct {
		Name string
		Age  int
	}

	userType := &ObjectType{
		Name_: "User",
		Fields_: FieldMap{
			"name": {Name_: "name", Type: StringScalar},
			"age":  {Name_: "age", Type: IntScalar},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"user": {
					Type: userType,
					Resolve: func(p ResolveParams) (interface{}, error) {
						return User{Name: "Alice", Age: 30}, nil
					},
				},
			},
		},
	})

	result := Do(schema, `{ user { name age } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	user := data["user"].(map[string]interface{})
	if user["name"] != "Alice" {
		t.Errorf("name = %v", user["name"])
	}
	if user["age"] != 30 {
		t.Errorf("age = %v", user["age"])
	}
}

func TestExecuteAbstractType(t *testing.T) {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name":   {Type: StringScalar},
			"height": {Type: FloatScalar},
		},
		IsTypeOf: func(value interface{}) bool {
			m, ok := value.(map[string]interface{})
			if !ok {
				return false
			}
			_, hasHeight := m["height"]
			return hasHeight
		},
	}

	droidType := &ObjectType{
		Name_: "Droid",
		Fields_: FieldMap{
			"name":      {Type: StringScalar},
			"primaryFn": {Type: StringScalar},
		},
		IsTypeOf: func(value interface{}) bool {
			m, ok := value.(map[string]interface{})
			if !ok {
				return false
			}
			_, hasPrimary := m["primaryFn"]
			return hasPrimary
		},
	}

	characterIface := &InterfaceType{
		Name_: "Character",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
	}

	humanType.Interfaces_ = []*InterfaceType{characterIface}
	droidType.Interfaces_ = []*InterfaceType{characterIface}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"hero": {
					Type: characterIface,
					Resolve: func(p ResolveParams) (interface{}, error) {
						return map[string]interface{}{"name": "R2-D2", "primaryFn": "Astromech"}, nil
					},
				},
			},
		},
		Types: []GraphQLType{humanType, droidType},
	})

	result := Do(schema, `{ hero { name __typename } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["__typename"] != "Droid" {
		t.Errorf("__typename = %v", hero["__typename"])
	}
}

func TestExecuteUnionType(t *testing.T) {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
	}

	union := &UnionType{
		Name_: "SearchResult",
		Types: []*ObjectType{humanType},
		ResolveType: func(value interface{}, info ResolveInfo) *ObjectType {
			return humanType
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"search": {
					Type: union,
					Resolve: func(p ResolveParams) (interface{}, error) {
						return map[string]interface{}{"name": "Luke"}, nil
					},
				},
			},
		},
	})

	result := Do(schema, `{ search { ... on Human { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	search := data["search"].(map[string]interface{})
	if search["name"] != "Luke" {
		t.Errorf("name = %v", search["name"])
	}
}

func TestExecuteInterfaceResolveType(t *testing.T) {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
	}

	characterIface := &InterfaceType{
		Name_: "Character",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
		ResolveType: func(value interface{}, info ResolveInfo) *ObjectType {
			return humanType
		},
	}

	humanType.Interfaces_ = []*InterfaceType{characterIface}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"hero": {
					Type: characterIface,
					Resolve: func(p ResolveParams) (interface{}, error) {
						return map[string]interface{}{"name": "Luke"}, nil
					},
				},
			},
		},
		Types: []GraphQLType{humanType},
	})

	result := Do(schema, `{ hero { name __typename } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["__typename"] != "Human" {
		t.Errorf("__typename = %v", hero["__typename"])
	}
}

func TestExecuteNoSchemaForMutation(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
	})
	result := Do(schema, `mutation { addItem }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected error for no mutation type")
	}
}

func TestExecuteFragment(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `
		{ hero { ...heroFields } }
		fragment heroFields on Human { name age }
	`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
}

func TestExecuteInlineFragment(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { ... on Human { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
}

func TestExecuteInlineFragmentNonMatching(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { ... on Droid { primaryFn } name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
	if _, ok := hero["primaryFn"]; ok {
		t.Error("primaryFn should not be present")
	}
}

func TestExecuteFieldMerging(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { name name } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if hero["name"] != "Luke" {
		t.Errorf("name = %v", hero["name"])
	}
}

func TestExecuteIntrospectionSchemaTypes(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __schema { types { name kind } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	types := schemaObj["types"].([]interface{})
	if len(types) == 0 {
		t.Error("expected types")
	}
}

func TestExecuteIntrospectionSchemaDirectives(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __schema { directives { name locations } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	directives := schemaObj["directives"].([]interface{})
	if len(directives) == 0 {
		t.Error("expected directives")
	}
}

func TestExecuteIntrospectionEnumType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __type(name: "Color") { name kind enumValues { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["kind"] != "ENUM" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionSchemaMutationType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __schema { mutationType { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	mt := schemaObj["mutationType"].(map[string]interface{})
	if mt["name"] != "Mutation" {
		t.Errorf("mutationType name = %v", mt["name"])
	}
}

func TestExecuteNilVariables(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hello }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestDoWithNilSchema(t *testing.T) {
	result := Do(nil, `{ hello }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected error for nil schema")
	}
}

func TestExecuteWithIntrospectionFragments(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__schema {
			queryType { ...typeFields }
		}
	}
	fragment typeFields on __Type { name }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	qt := schemaObj["queryType"].(map[string]interface{})
	if qt["name"] != "Query" {
		t.Errorf("name = %v", qt["name"])
	}
}

func TestExecuteSkipOnFragmentSpread(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `
		{ hero { ...fields @skip(if: true) } }
		fragment fields on Human { name }
	`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if _, ok := hero["name"]; ok {
		t.Error("name should be skipped")
	}
}

func TestExecuteSkipOnInlineFragment(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ hero { ... on Human @skip(if: true) { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	hero := data["hero"].(map[string]interface{})
	if _, ok := hero["name"]; ok {
		t.Error("name should be skipped")
	}
}

func TestExecuteAbstractTypeWithNoResolveType(t *testing.T) {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
	}

	characterIface := &InterfaceType{
		Name_: "Character",
		Fields_: FieldMap{
			"name": {Type: StringScalar},
		},
		// No ResolveType, no IsTypeOf
	}

	humanType.Interfaces_ = []*InterfaceType{characterIface}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"hero": {
					Type: characterIface,
					Resolve: func(p ResolveParams) (interface{}, error) {
						return map[string]interface{}{"name": "Luke"}, nil
					},
				},
			},
		},
		Types: []GraphQLType{humanType},
	})

	result := Do(schema, `{ hero { name } }`, nil, "")
	if !result.HasErrors() {
		t.Error("expected error for unresolvable abstract type")
	}
}

func TestExecuteIntrospectionFieldArgs(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__type(name: "Query") {
			fields {
				name
				args { name type { name kind } }
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteIntrospectionInputObject(t *testing.T) {
	inputType := &InputObjectType{
		Name_: "UserInput",
		Fields_: InputFieldMap{
			"name": {Type: NewNonNull(StringScalar)},
			"age":  {Type: IntScalar, DefaultValue: 25},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"user": {
					Type: StringScalar,
					Args: ArgumentMap{
						"input": {Type: inputType},
					},
					Resolve: func(p ResolveParams) (interface{}, error) {
						return "ok", nil
					},
				},
			},
		},
	})

	result := Do(schema, `{ __type(name: "UserInput") { name kind inputFields { name type { name kind } } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["kind"] != "INPUT_OBJECT" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionListAndNonNull(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__type(name: "Query") {
			fields {
				name
				type { name kind ofType { name kind } }
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteIntrospectionSchemaSubscriptionType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __schema { subscriptionType { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	if schemaObj["subscriptionType"] != nil {
		t.Error("subscriptionType should be nil")
	}
}

func TestExecuteIntrospectionUnionType(t *testing.T) {
	humanType := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{"name": {Type: StringScalar}},
	}
	union := &UnionType{
		Name_: "SearchResult",
		Types: []*ObjectType{humanType},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"search": {Type: union},
			},
		},
	})

	result := Do(schema, `{
		__type(name: "SearchResult") {
			name kind possibleTypes { name }
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["kind"] != "UNION" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionInterfaceType(t *testing.T) {
	iface := &InterfaceType{
		Name_: "Node",
		Fields_: FieldMap{
			"id": {Type: NewNonNull(IDScalar)},
		},
	}
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"node": {Type: iface},
			},
		},
	})

	result := Do(schema, `{
		__type(name: "Node") {
			name kind fields { name type { name kind } }
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["kind"] != "INTERFACE" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionScalarType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{ __type(name: "String") { name kind description } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	typeObj := data["__type"].(map[string]interface{})
	if typeObj["kind"] != "SCALAR" {
		t.Errorf("kind = %v", typeObj["kind"])
	}
}

func TestExecuteIntrospectionInlineFragmentOnType(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__schema {
			types {
				... on __Type { name kind }
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteIntrospectionDirectiveArgs(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__schema {
			directives {
				name
				args { name description type { name } defaultValue }
				isRepeatable
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteIntrospectionTypenameOnSchema(t *testing.T) {
	schema := heroSchema()
	result := Do(schema, `{
		__schema {
			__typename
			queryType { __typename name }
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	if schemaObj["__typename"] != "__Schema" {
		t.Errorf("__typename = %v", schemaObj["__typename"])
	}
}

func TestExecuteIntrospectionDeprecatedFields(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"old": {
					Type:              StringScalar,
					DeprecationReason: "Use 'new' instead",
				},
				"new": {Type: StringScalar},
			},
		},
	})

	result := Do(schema, `{
		__type(name: "Query") {
			fields {
				name isDeprecated deprecationReason
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteIntrospectionDeprecatedEnum(t *testing.T) {
	enumType := &EnumType{
		Name_: "Status",
		Values: []*EnumValueDefinition{
			{Name_: "ACTIVE"},
			{Name_: "DEPRECATED_VAL", DeprecationReason: "Use ACTIVE instead"},
		},
	}

	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{
				"status": {Type: enumType},
			},
		},
	})

	result := Do(schema, `{
		__type(name: "Status") {
			enumValues {
				name isDeprecated deprecationReason
			}
		}
	}`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
}

func TestExecuteWithSubscriptionType(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{
			Name_: "Query",
			Fields_: FieldMap{"q": {Type: StringScalar, Resolve: func(p ResolveParams) (interface{}, error) { return "ok", nil }}},
		},
		Subscription: &ObjectType{
			Name_: "Subscription",
			Fields_: FieldMap{"msg": {Type: StringScalar}},
		},
	})

	result := Do(schema, `{ __schema { subscriptionType { name } } }`, nil, "")
	if result.HasErrors() {
		t.Fatalf("errors: %v", FormatErrors(result.Errors))
	}
	data := result.Data.(map[string]interface{})
	schemaObj := data["__schema"].(map[string]interface{})
	st := schemaObj["subscriptionType"].(map[string]interface{})
	if st["name"] != "Subscription" {
		t.Errorf("subscriptionType name = %v", st["name"])
	}
}
