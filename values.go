package gqlx

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
)

// CoerceVariableValues coerces variable values according to their types.
func CoerceVariableValues(schema *Schema, varDefs []*VariableDefinition, inputs map[string]interface{}) (map[string]interface{}, []*GraphQLError) {
	coerced := make(map[string]interface{})
	var errors []*GraphQLError

	for _, varDef := range varDefs {
		varName := varDef.Variable
		varType := resolveASTType(schema, varDef.Type)
		if varType == nil {
			errors = append(errors, NewValidationError(
				fmt.Sprintf("Variable \"$%s\" expected value of type \"%s\" which cannot be used as an input type.", varName, varDef.Type),
				varDef.Loc))
			continue
		}

		value, hasValue := inputs[varName]
		if !hasValue {
			if varDef.DefaultValue != nil {
				coerced[varName] = valueFromAST(varDef.DefaultValue, varType, nil)
			} else if _, ok := varType.(*NonNullOfType); ok {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Variable \"$%s\" of required type \"%s\" was not provided.", varName, varType),
					varDef.Loc))
			}
			continue
		}

		if value == nil {
			if _, ok := varType.(*NonNullOfType); ok {
				errors = append(errors, NewValidationError(
					fmt.Sprintf("Variable \"$%s\" of non-null type \"%s\" must not be null.", varName, varType),
					varDef.Loc))
				continue
			}
			coerced[varName] = nil
			continue
		}

		coercedVal, err := coerceInputValue(value, varType)
		if err != nil {
			errors = append(errors, NewValidationError(
				fmt.Sprintf("Variable \"$%s\" got invalid value: %s", varName, err.Error()),
				varDef.Loc))
			continue
		}
		coerced[varName] = coercedVal
	}

	return coerced, errors
}

func resolveASTType(schema *Schema, astType Type) GraphQLType {
	switch t := astType.(type) {
	case *NamedType:
		return schema.Type(t.Name)
	case *ListType:
		inner := resolveASTType(schema, t.Type)
		if inner == nil {
			return nil
		}
		return NewList(inner)
	case *NonNullType:
		inner := resolveASTType(schema, t.Type)
		if inner == nil {
			return nil
		}
		return NewNonNull(inner)
	}
	return nil
}

func coerceInputValue(value interface{}, typ GraphQLType) (interface{}, error) {
	if nn, ok := typ.(*NonNullOfType); ok {
		if value == nil {
			return nil, fmt.Errorf("expected non-null value")
		}
		return coerceInputValue(value, nn.OfType)
	}

	if value == nil {
		return nil, nil
	}

	if list, ok := typ.(*ListOfType); ok {
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			result := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				item, err := coerceInputValue(rv.Index(i).Interface(), list.OfType)
				if err != nil {
					return nil, fmt.Errorf("in element #%d: %w", i, err)
				}
				result[i] = item
			}
			return result, nil
		}
		// Coerce single value to list
		item, err := coerceInputValue(value, list.OfType)
		if err != nil {
			return nil, err
		}
		return []interface{}{item}, nil
	}

	switch t := typ.(type) {
	case *ScalarType:
		return t.ParseValue(value)
	case *EnumType:
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("Enum \"%s\" cannot represent non-string value: %v", t.Name_, value)
		}
		for _, ev := range t.Values {
			if ev.Name_ == s {
				if ev.Value != nil {
					return ev.Value, nil
				}
				return s, nil
			}
		}
		return nil, fmt.Errorf("Value \"%s\" does not exist in \"%s\" enum", s, t.Name_)
	case *InputObjectType:
		m, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Expected object value for type \"%s\"", t.Name_)
		}
		result := make(map[string]interface{})
		for fieldName, fieldDef := range t.Fields_ {
			fieldVal, hasVal := m[fieldName]
			if !hasVal {
				if fieldDef.DefaultValue != nil {
					result[fieldName] = fieldDef.DefaultValue
				} else if _, isNonNull := fieldDef.Type.(*NonNullOfType); isNonNull {
					return nil, fmt.Errorf("Field \"%s.%s\" of required type \"%s\" was not provided", t.Name_, fieldName, fieldDef.Type)
				}
				continue
			}
			coerced, err := coerceInputValue(fieldVal, fieldDef.Type)
			if err != nil {
				return nil, fmt.Errorf("In field \"%s\": %w", fieldName, err)
			}
			result[fieldName] = coerced
		}
		return result, nil
	}

	return nil, fmt.Errorf("unexpected type: %s", typ)
}

// valueFromAST converts an AST value node to a Go value given a type.
func valueFromAST(valueNode Value, typ GraphQLType, variables map[string]interface{}) interface{} {
	if nn, ok := typ.(*NonNullOfType); ok {
		return valueFromAST(valueNode, nn.OfType, variables)
	}

	if valueNode == nil {
		return nil
	}

	if _, ok := valueNode.(*NullValue); ok {
		return nil
	}

	if varVal, ok := valueNode.(*VariableValue); ok {
		if variables != nil {
			return variables[varVal.Name]
		}
		return nil
	}

	if list, ok := typ.(*ListOfType); ok {
		if listVal, ok := valueNode.(*ListValue); ok {
			result := make([]interface{}, len(listVal.Values))
			for i, item := range listVal.Values {
				result[i] = valueFromAST(item, list.OfType, variables)
			}
			return result
		}
		// Coerce single value to list
		return []interface{}{valueFromAST(valueNode, list.OfType, variables)}
	}

	switch t := typ.(type) {
	case *ScalarType:
		if t.ParseLiteral != nil {
			val, err := t.ParseLiteral(valueNode)
			if err != nil {
				return nil
			}
			return val
		}
	case *EnumType:
		if ev, ok := valueNode.(*EnumValue); ok {
			for _, enumVal := range t.Values {
				if enumVal.Name_ == ev.Value {
					if enumVal.Value != nil {
						return enumVal.Value
					}
					return ev.Value
				}
			}
		}
		return nil
	case *InputObjectType:
		if obj, ok := valueNode.(*ObjectValue); ok {
			result := make(map[string]interface{})
			fieldMap := make(map[string]Value)
			for _, f := range obj.Fields {
				fieldMap[f.Name] = f.Value
			}
			for fieldName, fieldDef := range t.Fields_ {
				if fv, ok := fieldMap[fieldName]; ok {
					result[fieldName] = valueFromAST(fv, fieldDef.Type, variables)
				} else if fieldDef.DefaultValue != nil {
					result[fieldName] = fieldDef.DefaultValue
				}
			}
			return result
		}
	}

	// Fallback: extract Go value from AST value
	return astValueToGo(valueNode)
}

func astValueToGo(v Value) interface{} {
	switch val := v.(type) {
	case *IntValue:
		n, _ := strconv.Atoi(val.Value)
		return n
	case *FloatValue:
		f, _ := strconv.ParseFloat(val.Value, 64)
		return f
	case *StringValue:
		return val.Value
	case *BooleanValue:
		return val.Value
	case *NullValue:
		return nil
	case *EnumValue:
		return val.Value
	case *ListValue:
		result := make([]interface{}, len(val.Values))
		for i, item := range val.Values {
			result[i] = astValueToGo(item)
		}
		return result
	case *ObjectValue:
		result := make(map[string]interface{})
		for _, f := range val.Fields {
			result[f.Name] = astValueToGo(f.Value)
		}
		return result
	case *VariableValue:
		return nil
	}
	return nil
}

// CoerceArgumentValues coerces the arguments for a field or directive.
func CoerceArgumentValues(argDefs ArgumentMap, argNodes []*Argument, variables map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Build map of argument AST nodes
	nodeMap := make(map[string]*Argument)
	for _, arg := range argNodes {
		nodeMap[arg.Name] = arg
	}

	for argName, argDef := range argDefs {
		argNode, hasNode := nodeMap[argName]
		var argValue interface{}
		hasValue := false

		if hasNode {
			if varVal, ok := argNode.Value.(*VariableValue); ok {
				val, exists := variables[varVal.Name]
				if exists {
					argValue = val
					hasValue = true
				}
			} else {
				argValue = valueFromAST(argNode.Value, argDef.Type, variables)
				hasValue = true
			}
		}

		if !hasValue {
			if argDef.DefaultValue != nil {
				result[argName] = argDef.DefaultValue
			} else if _, isNonNull := argDef.Type.(*NonNullOfType); isNonNull {
				return nil, fmt.Errorf("Argument \"%s\" of required type \"%s\" was not provided", argName, argDef.Type)
			}
			continue
		}

		if argValue == nil {
			if _, isNonNull := argDef.Type.(*NonNullOfType); isNonNull {
				return nil, fmt.Errorf("Argument \"%s\" of non-null type \"%s\" must not be null", argName, argDef.Type)
			}
			result[argName] = nil
			continue
		}

		result[argName] = argValue
	}

	return result, nil
}

// resolveFieldValue gets a value from a source for a given field name using reflection.
func resolveFieldValue(source interface{}, fieldName string) interface{} {
	if source == nil {
		return nil
	}
	if m, ok := source.(map[string]interface{}); ok {
		return m[fieldName]
	}

	rv := reflect.ValueOf(source)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Map {
		val := rv.MapIndex(reflect.ValueOf(fieldName))
		if val.IsValid() {
			return val.Interface()
		}
		return nil
	}
	if rv.Kind() == reflect.Struct {
		// Try exact field name match
		field := rv.FieldByName(fieldName)
		if field.IsValid() {
			return field.Interface()
		}
		// Try case-insensitive match
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag != "" {
				// Parse json tag
				name := tag
				if idx := indexOf(tag, ','); idx != -1 {
					name = tag[:idx]
				}
				if name == fieldName {
					return rv.Field(i).Interface()
				}
			}
			if equalFold(f.Name, fieldName) {
				return rv.Field(i).Interface()
			}
		}
	}
	return nil
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// CompleteValue serializes a resolved value according to its type.
func CompleteValue(typ GraphQLType, value interface{}) (interface{}, error) {
	if nn, ok := typ.(*NonNullOfType); ok {
		completed, err := CompleteValue(nn.OfType, value)
		if err != nil {
			return nil, err
		}
		if completed == nil {
			return nil, fmt.Errorf("Cannot return null for non-nullable field")
		}
		return completed, nil
	}

	if value == nil {
		return nil, nil
	}

	if list, ok := typ.(*ListOfType); ok {
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil, fmt.Errorf("expected iterable, got %T", value)
		}
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			item, err := CompleteValue(list.OfType, rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			result[i] = item
		}
		return result, nil
	}

	if scalar, ok := typ.(*ScalarType); ok {
		return scalar.Serialize(value)
	}

	if enum, ok := typ.(*EnumType); ok {
		s := fmt.Sprintf("%v", value)
		for _, ev := range enum.Values {
			val := ev.Name_
			if ev.Value != nil {
				val = fmt.Sprintf("%v", ev.Value)
			}
			if val == s || ev.Name_ == s {
				return ev.Name_, nil
			}
		}
		return nil, fmt.Errorf("Enum \"%s\" cannot represent value: %v", enum.Name_, value)
	}

	return value, nil
}

// CoerceResultToList converts the result to a list if needed.
func CoerceResultToList(value interface{}) []interface{} {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		result := make([]interface{}, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}
	return nil
}

// isIntegerValue checks if a float64 is actually an integer value
func isIntegerValue(f float64) bool {
	return f == math.Trunc(f) && !math.IsInf(f, 0) && !math.IsNaN(f)
}
