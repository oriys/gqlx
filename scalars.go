package gqlx

import (
	"fmt"
	"math"
	"strconv"
)

// Built-in scalar types
var (
	IntScalar = &ScalarType{
		Name_:       "Int",
		Description: "The `Int` scalar type represents non-fractional signed whole numeric values.",
		Serialize: func(value interface{}) (interface{}, error) {
			return coerceInt(value)
		},
		ParseValue: func(value interface{}) (interface{}, error) {
			return coerceInt(value)
		},
		ParseLiteral: func(value Value) (interface{}, error) {
			switch v := value.(type) {
			case *IntValue:
				n, err := strconv.Atoi(v.Value)
				if err != nil {
					return nil, fmt.Errorf("Int cannot represent non-integer value: %s", v.Value)
				}
				if n > math.MaxInt32 || n < math.MinInt32 {
					return nil, fmt.Errorf("Int cannot represent non 32-bit signed integer value: %s", v.Value)
				}
				return n, nil
			default:
				return nil, fmt.Errorf("Int cannot represent non-integer value")
			}
		},
	}

	FloatScalar = &ScalarType{
		Name_:       "Float",
		Description: "The `Float` scalar type represents signed double-precision fractional values.",
		Serialize: func(value interface{}) (interface{}, error) {
			return coerceFloat(value)
		},
		ParseValue: func(value interface{}) (interface{}, error) {
			return coerceFloat(value)
		},
		ParseLiteral: func(value Value) (interface{}, error) {
			switch v := value.(type) {
			case *FloatValue:
				f, err := strconv.ParseFloat(v.Value, 64)
				if err != nil {
					return nil, fmt.Errorf("Float cannot represent non-numeric value: %s", v.Value)
				}
				return f, nil
			case *IntValue:
				f, err := strconv.ParseFloat(v.Value, 64)
				if err != nil {
					return nil, fmt.Errorf("Float cannot represent non-numeric value: %s", v.Value)
				}
				return f, nil
			default:
				return nil, fmt.Errorf("Float cannot represent non-numeric value")
			}
		},
	}

	StringScalar = &ScalarType{
		Name_:       "String",
		Description: "The `String` scalar type represents textual data, represented as UTF-8 character sequences.",
		Serialize: func(value interface{}) (interface{}, error) {
			return coerceString(value)
		},
		ParseValue: func(value interface{}) (interface{}, error) {
			s, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("String cannot represent a non string value")
			}
			return s, nil
		},
		ParseLiteral: func(value Value) (interface{}, error) {
			switch v := value.(type) {
			case *StringValue:
				return v.Value, nil
			default:
				return nil, fmt.Errorf("String cannot represent a non string value")
			}
		},
	}

	BooleanScalar = &ScalarType{
		Name_:       "Boolean",
		Description: "The `Boolean` scalar type represents `true` or `false`.",
		Serialize: func(value interface{}) (interface{}, error) {
			return coerceBool(value)
		},
		ParseValue: func(value interface{}) (interface{}, error) {
			b, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("Boolean cannot represent a non boolean value")
			}
			return b, nil
		},
		ParseLiteral: func(value Value) (interface{}, error) {
			switch v := value.(type) {
			case *BooleanValue:
				return v.Value, nil
			default:
				return nil, fmt.Errorf("Boolean cannot represent a non boolean value")
			}
		},
	}

	IDScalar = &ScalarType{
		Name_:       "ID",
		Description: "The `ID` scalar type represents a unique identifier.",
		Serialize: func(value interface{}) (interface{}, error) {
			return coerceString(value)
		},
		ParseValue: func(value interface{}) (interface{}, error) {
			switch v := value.(type) {
			case string:
				return v, nil
			case int:
				return strconv.Itoa(v), nil
			default:
				return nil, fmt.Errorf("ID cannot represent value: %v", value)
			}
		},
		ParseLiteral: func(value Value) (interface{}, error) {
			switch v := value.(type) {
			case *StringValue:
				return v.Value, nil
			case *IntValue:
				return v.Value, nil
			default:
				return nil, fmt.Errorf("ID cannot represent a non-string and non-integer value")
			}
		},
	}
)

func coerceInt(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		if v > math.MaxInt32 || v < math.MinInt32 {
			return nil, fmt.Errorf("Int cannot represent non 32-bit signed integer value: %d", v)
		}
		return int(v), nil
	case float64:
		if v != math.Trunc(v) {
			return nil, fmt.Errorf("Int cannot represent non-integer value: %v", v)
		}
		if v > math.MaxInt32 || v < math.MinInt32 {
			return nil, fmt.Errorf("Int cannot represent non 32-bit signed integer value: %v", v)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("Int cannot represent non-integer value: %q", v)
		}
		return n, nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("Int cannot represent non-integer value: %v", value)
}

func coerceFloat(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("Float cannot represent non-numeric value: %q", v)
		}
		return f, nil
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("Float cannot represent non-numeric value: %v", value)
}

func coerceString(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	case nil:
		return nil, nil
	}
	return fmt.Sprintf("%v", value), nil
}

func coerceBool(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		return v != 0, nil
	case float64:
		return v != 0, nil
	case string:
		return v != "", nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("Boolean cannot represent a non boolean value: %v", value)
}
