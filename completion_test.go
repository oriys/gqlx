package gqlx

import (
	"fmt"
	"testing"
)

// nonNullSchema builds a schema used to exercise null propagation.
func nonNullSchema() *Schema {
	human := &ObjectType{
		Name_: "Human",
		Fields_: FieldMap{
			"name": {Type: NewNonNull(StringScalar)},
			"age":  {Type: IntScalar},
		},
	}
	query := &ObjectType{
		Name_: "Query",
		Fields_: FieldMap{
			// Nullable object containing a non-null child.
			"hero": {
				Type: human,
				Resolve: func(p ResolveParams) (interface{}, error) {
					return map[string]interface{}{"name": nil, "age": 30}, nil
				},
			},
			// Non-null object containing a non-null child.
			"heroStrict": {
				Type: NewNonNull(human),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return map[string]interface{}{"name": nil, "age": 30}, nil
				},
			},
			// Nullable list of nullable objects.
			"heroes": {
				Type: NewList(human),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return []interface{}{
						map[string]interface{}{"name": "Luke", "age": 25},
						map[string]interface{}{"name": nil, "age": 0},
						map[string]interface{}{"name": "Leia", "age": 25},
					}, nil
				},
			},
			// Nullable list of non-null objects.
			"heroesStrict": {
				Type: NewList(NewNonNull(human)),
				Resolve: func(p ResolveParams) (interface{}, error) {
					return []interface{}{
						map[string]interface{}{"name": "Luke", "age": 25},
						map[string]interface{}{"name": nil, "age": 0},
					}, nil
				},
			},
			"ok": {
				Type:    StringScalar,
				Resolve: func(p ResolveParams) (interface{}, error) { return "yes", nil },
			},
		},
	}
	schema, _ := NewSchema(SchemaConfig{Query: query})
	return schema
}

func TestNullPropagationNullableObjectBecomesNull(t *testing.T) {
	result := Do(nonNullSchema(), `{ hero { name age } }`, nil, "")
	if !result.HasErrors() {
		t.Fatal("expected an error")
	}
	data := result.Data.(map[string]interface{})
	if data["hero"] != nil {
		t.Fatalf("hero should be null, got %#v", data["hero"])
	}
	if got := result.Errors[0].Message; got != "Cannot return null for non-nullable field." {
		t.Fatalf("error message = %q", got)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("expected exactly 1 error, got %d", len(result.Errors))
	}
}

func TestNullPropagationNonNullRootBecomesNullData(t *testing.T) {
	result := Do(nonNullSchema(), `{ heroStrict { name age } }`, nil, "")
	if !result.HasErrors() {
		t.Fatal("expected an error")
	}
	if result.Data != nil {
		t.Fatalf("data should be null, got %#v", result.Data)
	}
}

func TestNullPropagationNullableListItemBecomesNull(t *testing.T) {
	result := Do(nonNullSchema(), `{ heroes { name } }`, nil, "")
	if !result.HasErrors() {
		t.Fatal("expected an error")
	}
	data := result.Data.(map[string]interface{})
	items := data["heroes"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0] == nil || items[2] == nil {
		t.Fatalf("valid items should not be null: %#v", items)
	}
	if items[1] != nil {
		t.Fatalf("failing item should be null, got %#v", items[1])
	}
}

func TestNullPropagationNonNullListItemNullsList(t *testing.T) {
	result := Do(nonNullSchema(), `{ heroesStrict { name } }`, nil, "")
	if !result.HasErrors() {
		t.Fatal("expected an error")
	}
	data := result.Data.(map[string]interface{})
	if data["heroesStrict"] != nil {
		t.Fatalf("list should be null, got %#v", data["heroesStrict"])
	}
}

func TestNullPropagationErrorMessageAndPath(t *testing.T) {
	result := Do(nonNullSchema(), `{ hero { name } }`, nil, "")
	if len(result.Errors) == 0 {
		t.Fatal("expected an error")
	}
	path := result.Errors[0].Path
	if len(path) != 2 || path[0] != "hero" || path[1] != "name" {
		t.Fatalf("unexpected path: %#v", path)
	}
}

func TestConcurrentExecutionMatchesSerial(t *testing.T) {
	schema := nonNullSchema()
	query := `{ ok heroes { name } }`

	serial := Execute(ExecuteParams{
		Schema:   schema,
		Document: mustParse(t, query),
	})
	concurrent := Execute(ExecuteParams{
		Schema:     schema,
		Document:   mustParse(t, query),
		Concurrent: true,
	})

	if serial.HasErrors() != concurrent.HasErrors() {
		t.Fatalf("error mismatch: serial=%v concurrent=%v", serial.Errors, concurrent.Errors)
	}
	if fmt.Sprintf("%v", serial.Data) != fmt.Sprintf("%v", concurrent.Data) {
		t.Fatalf("data mismatch: serial=%v concurrent=%v", serial.Data, concurrent.Data)
	}
}

func mustParse(t *testing.T, query string) *Document {
	t.Helper()
	doc, err := Parse(query)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return doc
}
