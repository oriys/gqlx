package gqlx

import (
	"testing"
)

func TestResolveFieldValue(t *testing.T) {
	// Map
	m := map[string]interface{}{"name": "Luke", "age": 25}
	if resolveFieldValue(m, "name") != "Luke" {
		t.Error("map field resolution failed")
	}
	if resolveFieldValue(m, "missing") != nil {
		t.Error("missing map field should return nil")
	}

	// Struct
	type User struct {
		Name string
		Age  int
	}
	u := User{Name: "Luke", Age: 25}
	if resolveFieldValue(u, "Name") != "Luke" {
		t.Error("struct field resolution failed")
	}

	// Struct with json tag
	type TaggedUser struct {
		FullName string `json:"name"`
	}
	tu := TaggedUser{FullName: "Luke"}
	if resolveFieldValue(tu, "name") != "Luke" {
		t.Error("json tag field resolution failed")
	}

	// Case-insensitive match
	if resolveFieldValue(u, "name") != "Luke" {
		t.Error("case-insensitive field resolution failed")
	}

	// Nil source
	if resolveFieldValue(nil, "name") != nil {
		t.Error("nil source should return nil")
	}

	// Pointer
	if resolveFieldValue(&u, "Name") != "Luke" {
		t.Error("pointer struct field resolution failed")
	}

	// Nil pointer
	var p *User
	if resolveFieldValue(p, "Name") != nil {
		t.Error("nil pointer should return nil")
	}

	// Reflect map
	rm := map[string]string{"key": "value"}
	if resolveFieldValue(rm, "key") != "value" {
		t.Error("reflect map resolution failed")
	}
	if resolveFieldValue(rm, "missing") != nil {
		t.Error("missing reflect map key should return nil")
	}
}

func TestResolveFieldValueStructTag(t *testing.T) {
	type Item struct {
		ItemName string `json:"item_name,omitempty"`
	}
	item := Item{ItemName: "test"}
	if resolveFieldValue(item, "item_name") != "test" {
		t.Error("json tag with options resolution failed")
	}
}

func TestCoerceInputValue(t *testing.T) {
	// Scalar
	val, err := coerceInputValue(42, IntScalar)
	if err != nil || val != 42 {
		t.Errorf("coerceInputValue(42, Int) = %v, %v", val, err)
	}

	// NonNull
	_, err = coerceInputValue(nil, NewNonNull(IntScalar))
	if err == nil {
		t.Error("expected error for null non-null")
	}

	// List
	val, err = coerceInputValue([]interface{}{1, 2, 3}, NewList(IntScalar))
	if err != nil {
		t.Errorf("list coercion error: %v", err)
	}
	arr := val.([]interface{})
	if len(arr) != 3 {
		t.Errorf("list length = %d", len(arr))
	}

	// Single value to list
	val, err = coerceInputValue(42, NewList(IntScalar))
	if err != nil {
		t.Errorf("single to list error: %v", err)
	}
	arr = val.([]interface{})
	if len(arr) != 1 || arr[0] != 42 {
		t.Error("single to list coercion failed")
	}

	// Enum
	enumType := &EnumType{
		Name_: "Color",
		Values: []*EnumValueDefinition{
			{Name_: "RED"},
			{Name_: "GREEN"},
		},
	}
	val, err = coerceInputValue("RED", enumType)
	if err != nil || val != "RED" {
		t.Errorf("enum coercion = %v, %v", val, err)
	}
	_, err = coerceInputValue("YELLOW", enumType)
	if err == nil {
		t.Error("expected error for invalid enum")
	}
	_, err = coerceInputValue(42, enumType)
	if err == nil {
		t.Error("expected error for non-string enum")
	}

	// InputObject
	inputType := &InputObjectType{
		Name_: "UserInput",
		Fields_: InputFieldMap{
			"name": {Type: NewNonNull(StringScalar)},
			"age":  {Type: IntScalar, DefaultValue: 0},
		},
	}
	val, err = coerceInputValue(map[string]interface{}{"name": "Luke"}, inputType)
	if err != nil {
		t.Errorf("input object coercion error: %v", err)
	}
	obj := val.(map[string]interface{})
	if obj["name"] != "Luke" {
		t.Error("input object name mismatch")
	}

	// Missing required field
	_, err = coerceInputValue(map[string]interface{}{}, inputType)
	if err == nil {
		t.Error("expected error for missing required field")
	}

	// Wrong type for input object
	_, err = coerceInputValue("not an object", inputType)
	if err == nil {
		t.Error("expected error for non-object")
	}

	// Null for nullable
	val, err = coerceInputValue(nil, IntScalar)
	if err != nil || val != nil {
		t.Errorf("null nullable coercion = %v, %v", val, err)
	}

	// List element error
	_, err = coerceInputValue([]interface{}{"not_int"}, NewList(IntScalar))
	if err == nil {
		t.Error("expected error for invalid list element")
	}
}

func TestCoerceInputValueEnumWithValue(t *testing.T) {
	enumType := &EnumType{
		Name_: "Status",
		Values: []*EnumValueDefinition{
			{Name_: "ACTIVE", Value: 1},
			{Name_: "INACTIVE", Value: 0},
		},
	}
	val, err := coerceInputValue("ACTIVE", enumType)
	if err != nil || val != 1 {
		t.Errorf("enum with Value = %v, %v", val, err)
	}
}

func TestValueFromAST(t *testing.T) {
	// Int
	val := valueFromAST(&IntValue{Value: "42"}, IntScalar, nil)
	if val != 42 {
		t.Errorf("IntValue -> %v", val)
	}

	// Float
	val = valueFromAST(&FloatValue{Value: "3.14"}, FloatScalar, nil)
	if val.(float64) != 3.14 {
		t.Errorf("FloatValue -> %v", val)
	}

	// String
	val = valueFromAST(&StringValue{Value: "hello"}, StringScalar, nil)
	if val != "hello" {
		t.Errorf("StringValue -> %v", val)
	}

	// Boolean
	val = valueFromAST(&BooleanValue{Value: true}, BooleanScalar, nil)
	if val != true {
		t.Errorf("BooleanValue -> %v", val)
	}

	// Null
	val = valueFromAST(&NullValue{}, StringScalar, nil)
	if val != nil {
		t.Errorf("NullValue -> %v", val)
	}

	// Nil node
	val = valueFromAST(nil, StringScalar, nil)
	if val != nil {
		t.Errorf("nil -> %v", val)
	}

	// Variable
	vars := map[string]interface{}{"x": 42}
	val = valueFromAST(&VariableValue{Name: "x"}, IntScalar, vars)
	if val != 42 {
		t.Errorf("Variable -> %v", val)
	}

	// Variable without variables map
	val = valueFromAST(&VariableValue{Name: "x"}, IntScalar, nil)
	if val != nil {
		t.Errorf("Variable without vars -> %v", val)
	}

	// List
	val = valueFromAST(&ListValue{Values: []Value{
		&IntValue{Value: "1"}, &IntValue{Value: "2"},
	}}, NewList(IntScalar), nil)
	arr := val.([]interface{})
	if len(arr) != 2 || arr[0] != 1 || arr[1] != 2 {
		t.Errorf("ListValue -> %v", val)
	}

	// Single value coerced to list
	val = valueFromAST(&IntValue{Value: "5"}, NewList(IntScalar), nil)
	arr = val.([]interface{})
	if len(arr) != 1 || arr[0] != 5 {
		t.Errorf("Single to list -> %v", val)
	}

	// Enum
	enumType := &EnumType{
		Name_: "Color",
		Values: []*EnumValueDefinition{
			{Name_: "RED"},
		},
	}
	val = valueFromAST(&EnumValue{Value: "RED"}, enumType, nil)
	if val != "RED" {
		t.Errorf("EnumValue -> %v", val)
	}
	val = valueFromAST(&EnumValue{Value: "INVALID"}, enumType, nil)
	if val != nil {
		t.Errorf("Invalid enum -> %v", val)
	}

	// InputObject
	inputType := &InputObjectType{
		Name_: "Point",
		Fields_: InputFieldMap{
			"x": {Type: IntScalar},
			"y": {Type: IntScalar, DefaultValue: 0},
		},
	}
	val = valueFromAST(&ObjectValue{
		Fields: []*ObjectField{{Name: "x", Value: &IntValue{Value: "1"}}},
	}, inputType, nil)
	obj := val.(map[string]interface{})
	if obj["x"] != 1 || obj["y"] != 0 {
		t.Errorf("InputObject -> %v", val)
	}

	// NonNull wrapping
	val = valueFromAST(&IntValue{Value: "42"}, NewNonNull(IntScalar), nil)
	if val != 42 {
		t.Errorf("NonNull -> %v", val)
	}
}

func TestCoerceArgumentValues(t *testing.T) {
	argDefs := ArgumentMap{
		"name": {Name_: "name", Type: NewNonNull(StringScalar)},
		"age":  {Name_: "age", Type: IntScalar, DefaultValue: 25},
	}

	args := []*Argument{
		{Name: "name", Value: &StringValue{Value: "Luke"}},
	}

	result, err := CoerceArgumentValues(argDefs, args, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["name"] != "Luke" {
		t.Errorf("name = %v", result["name"])
	}
	if result["age"] != 25 {
		t.Errorf("age = %v, want 25", result["age"])
	}
}

func TestCoerceArgumentValuesMissingRequired(t *testing.T) {
	argDefs := ArgumentMap{
		"name": {Name_: "name", Type: NewNonNull(StringScalar)},
	}
	_, err := CoerceArgumentValues(argDefs, nil, nil)
	if err == nil {
		t.Error("expected error for missing required argument")
	}
}

func TestCoerceArgumentValuesNullNonNull(t *testing.T) {
	argDefs := ArgumentMap{
		"name": {Name_: "name", Type: NewNonNull(StringScalar)},
	}
	args := []*Argument{
		{Name: "name", Value: &NullValue{}},
	}
	_, err := CoerceArgumentValues(argDefs, args, nil)
	if err == nil {
		t.Error("expected error for null non-null argument")
	}
}

func TestCoerceArgumentValuesVariable(t *testing.T) {
	argDefs := ArgumentMap{
		"id": {Name_: "id", Type: IDScalar},
	}
	args := []*Argument{
		{Name: "id", Value: &VariableValue{Name: "myId"}},
	}
	vars := map[string]interface{}{"myId": "123"}
	result, err := CoerceArgumentValues(argDefs, args, vars)
	if err != nil {
		t.Fatal(err)
	}
	if result["id"] != "123" {
		t.Errorf("id = %v", result["id"])
	}
}

func TestCoerceArgumentValuesUndefinedVariable(t *testing.T) {
	argDefs := ArgumentMap{
		"id": {Name_: "id", Type: IDScalar, DefaultValue: "default"},
	}
	args := []*Argument{
		{Name: "id", Value: &VariableValue{Name: "missing"}},
	}
	result, err := CoerceArgumentValues(argDefs, args, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if result["id"] != "default" {
		t.Errorf("id = %v, want 'default'", result["id"])
	}
}

func TestCoerceVariableValues(t *testing.T) {
	schema, _ := NewSchema(SchemaConfig{
		Query: &ObjectType{Name_: "Query", Fields_: FieldMap{"q": {Type: StringScalar}}},
	})

	varDefs := []*VariableDefinition{
		{Variable: "name", Type: &NonNullType{Type: &NamedType{Name: "String"}}},
		{Variable: "age", Type: &NamedType{Name: "Int"}, DefaultValue: &IntValue{Value: "25"}},
	}

	// Valid
	coerced, errs := CoerceVariableValues(schema, varDefs, map[string]interface{}{"name": "Luke"})
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	if coerced["name"] != "Luke" {
		t.Errorf("name = %v", coerced["name"])
	}
	if coerced["age"] != 25 {
		t.Errorf("age = %v", coerced["age"])
	}

	// Missing required
	_, errs = CoerceVariableValues(schema, varDefs, map[string]interface{}{})
	if len(errs) == 0 {
		t.Error("expected error for missing required variable")
	}

	// Null for non-null
	_, errs = CoerceVariableValues(schema, varDefs, map[string]interface{}{"name": nil})
	if len(errs) == 0 {
		t.Error("expected error for null non-null variable")
	}

	// Unknown type
	badVarDefs := []*VariableDefinition{
		{Variable: "x", Type: &NamedType{Name: "Unknown"}},
	}
	_, errs = CoerceVariableValues(schema, badVarDefs, map[string]interface{}{"x": 1})
	if len(errs) == 0 {
		t.Error("expected error for unknown type")
	}
}

func TestCompleteValue(t *testing.T) {
	// Scalar
	val, err := CompleteValue(StringScalar, "hello")
	if err != nil || val != "hello" {
		t.Errorf("CompleteValue(String, hello) = %v, %v", val, err)
	}

	// Null
	val, err = CompleteValue(StringScalar, nil)
	if err != nil || val != nil {
		t.Errorf("CompleteValue(String, nil) = %v, %v", val, err)
	}

	// NonNull null
	_, err = CompleteValue(NewNonNull(StringScalar), nil)
	if err == nil {
		t.Error("expected error for null non-null")
	}

	// List
	val, err = CompleteValue(NewList(IntScalar), []int{1, 2, 3})
	if err != nil {
		t.Errorf("CompleteValue list error: %v", err)
	}
	arr := val.([]interface{})
	if len(arr) != 3 {
		t.Errorf("list length = %d", len(arr))
	}

	// Non-iterable for list
	_, err = CompleteValue(NewList(IntScalar), "not a list")
	if err == nil {
		t.Error("expected error for non-iterable list")
	}

	// Enum
	enumType := &EnumType{
		Name_:  "Status",
		Values: []*EnumValueDefinition{{Name_: "ACTIVE", Value: 1}},
	}
	val, err = CompleteValue(enumType, 1)
	if err != nil || val != "ACTIVE" {
		t.Errorf("CompleteValue enum = %v, %v", val, err)
	}
	_, err = CompleteValue(enumType, 999)
	if err == nil {
		t.Error("expected error for invalid enum value")
	}
}

func TestCoerceResultToList(t *testing.T) {
	result := CoerceResultToList([]int{1, 2, 3})
	if len(result) != 3 {
		t.Errorf("list length = %d", len(result))
	}

	if CoerceResultToList(nil) != nil {
		t.Error("nil should return nil")
	}

	if CoerceResultToList("not a list") != nil {
		t.Error("non-list should return nil")
	}
}

func TestASTValueToGo(t *testing.T) {
	if astValueToGo(&VariableValue{Name: "x"}) != nil {
		t.Error("VariableValue should return nil")
	}
	
	obj := astValueToGo(&ObjectValue{
		Fields: []*ObjectField{{Name: "a", Value: &IntValue{Value: "1"}}},
	})
	if m, ok := obj.(map[string]interface{}); !ok || m["a"] != 1 {
		t.Errorf("ObjectValue astValueToGo = %v", obj)
	}
}

func TestEqualFold(t *testing.T) {
	if !equalFold("Hello", "hello") {
		t.Error("Hello should equalFold hello")
	}
	if equalFold("Hello", "world") {
		t.Error("Hello should not equalFold world")
	}
	if equalFold("a", "ab") {
		t.Error("different lengths should not equalFold")
	}
}

func TestIsIntegerValue(t *testing.T) {
	if !isIntegerValue(5.0) {
		t.Error("5.0 should be integer")
	}
	if isIntegerValue(5.5) {
		t.Error("5.5 should not be integer")
	}
}
