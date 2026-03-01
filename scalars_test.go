package gqlx

import (
	"testing"
)

func TestIntScalar(t *testing.T) {
	// Serialize
	tests := []struct {
		input interface{}
		want  interface{}
		err   bool
	}{
		{42, 42, false},
		{int8(1), 1, false},
		{int16(2), 2, false},
		{int32(3), 3, false},
		{int64(4), 4, false},
		{float64(5), 5, false},
		{float64(5.5), nil, true},
		{"10", 10, false},
		{true, 1, false},
		{false, 0, false},
		{nil, nil, false},
		{struct{}{}, nil, true},
	}
	for _, tt := range tests {
		got, err := IntScalar.Serialize(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("IntScalar.Serialize(%v) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if !tt.err && got != tt.want {
			t.Errorf("IntScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}

	// ParseLiteral
	val, err := IntScalar.ParseLiteral(&IntValue{Value: "42"})
	if err != nil || val != 42 {
		t.Errorf("ParseLiteral(42) = %v, %v", val, err)
	}

	_, err = IntScalar.ParseLiteral(&StringValue{Value: "not int"})
	if err == nil {
		t.Error("expected error for non-int literal")
	}

	_, err = IntScalar.ParseLiteral(&IntValue{Value: "99999999999"})
	if err == nil {
		t.Error("expected error for overflow")
	}

	// ParseValue
	val, err = IntScalar.ParseValue(42)
	if err != nil || val != 42 {
		t.Errorf("ParseValue(42) = %v, %v", val, err)
	}
}

func TestFloatScalar(t *testing.T) {
	// Serialize
	tests := []struct {
		input interface{}
		want  interface{}
		err   bool
	}{
		{3.14, 3.14, false},
		{float32(1.5), float64(float32(1.5)), false},
		{42, float64(42), false},
		{int32(1), float64(1), false},
		{int64(2), float64(2), false},
		{"3.14", 3.14, false},
		{true, 1.0, false},
		{false, 0.0, false},
		{nil, nil, false},
		{struct{}{}, nil, true},
	}
	for _, tt := range tests {
		got, err := FloatScalar.Serialize(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("FloatScalar.Serialize(%v) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if !tt.err && got != tt.want {
			t.Errorf("FloatScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}

	// ParseLiteral
	val, err := FloatScalar.ParseLiteral(&FloatValue{Value: "3.14"})
	if err != nil {
		t.Errorf("ParseLiteral float error: %v", err)
	}
	if val.(float64) != 3.14 {
		t.Errorf("ParseLiteral(3.14) = %v", val)
	}

	val, err = FloatScalar.ParseLiteral(&IntValue{Value: "42"})
	if err != nil || val.(float64) != 42 {
		t.Errorf("ParseLiteral int as float: %v, %v", val, err)
	}

	_, err = FloatScalar.ParseLiteral(&StringValue{Value: "nope"})
	if err == nil {
		t.Error("expected error")
	}
}

func TestStringScalar(t *testing.T) {
	// Serialize
	tests := []struct {
		input interface{}
		want  interface{}
	}{
		{"hello", "hello"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
		{nil, nil},
	}
	for _, tt := range tests {
		got, _ := StringScalar.Serialize(tt.input)
		if got != tt.want {
			t.Errorf("StringScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}

	// ParseValue
	val, err := StringScalar.ParseValue("hello")
	if err != nil || val != "hello" {
		t.Errorf("ParseValue = %v, %v", val, err)
	}
	_, err = StringScalar.ParseValue(42)
	if err == nil {
		t.Error("expected error for non-string")
	}

	// ParseLiteral
	val, err = StringScalar.ParseLiteral(&StringValue{Value: "hi"})
	if err != nil || val != "hi" {
		t.Errorf("ParseLiteral = %v, %v", val, err)
	}
	_, err = StringScalar.ParseLiteral(&IntValue{Value: "1"})
	if err == nil {
		t.Error("expected error for non-string literal")
	}
}

func TestBooleanScalar(t *testing.T) {
	// Serialize
	tests := []struct {
		input interface{}
		want  interface{}
		err   bool
	}{
		{true, true, false},
		{false, false, false},
		{1, true, false},
		{0, false, false},
		{1.0, true, false},
		{0.0, false, false},
		{"hello", true, false},
		{"", false, false},
		{nil, nil, false},
		{struct{}{}, nil, true},
	}
	for _, tt := range tests {
		got, err := BooleanScalar.Serialize(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("BooleanScalar.Serialize(%v) error = %v, wantErr %v", tt.input, err, tt.err)
			continue
		}
		if !tt.err && got != tt.want {
			t.Errorf("BooleanScalar.Serialize(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}

	// ParseValue
	val, err := BooleanScalar.ParseValue(true)
	if err != nil || val != true {
		t.Errorf("ParseValue = %v, %v", val, err)
	}
	_, err = BooleanScalar.ParseValue("true")
	if err == nil {
		t.Error("expected error for non-bool")
	}

	// ParseLiteral
	val, err = BooleanScalar.ParseLiteral(&BooleanValue{Value: true})
	if err != nil || val != true {
		t.Errorf("ParseLiteral = %v, %v", val, err)
	}
	_, err = BooleanScalar.ParseLiteral(&StringValue{Value: "true"})
	if err == nil {
		t.Error("expected error for non-bool literal")
	}
}

func TestIDScalar(t *testing.T) {
	// Serialize (same as string)
	got, _ := IDScalar.Serialize("abc")
	if got != "abc" {
		t.Errorf("IDScalar.Serialize(abc) = %v", got)
	}
	got, _ = IDScalar.Serialize(42)
	if got != "42" {
		t.Errorf("IDScalar.Serialize(42) = %v", got)
	}

	// ParseValue
	val, err := IDScalar.ParseValue("abc")
	if err != nil || val != "abc" {
		t.Errorf("ParseValue(abc) = %v, %v", val, err)
	}
	val, err = IDScalar.ParseValue(42)
	if err != nil || val != "42" {
		t.Errorf("ParseValue(42) = %v, %v", val, err)
	}
	_, err = IDScalar.ParseValue(3.14)
	if err == nil {
		t.Error("expected error for float as ID")
	}

	// ParseLiteral
	val, err = IDScalar.ParseLiteral(&StringValue{Value: "id1"})
	if err != nil || val != "id1" {
		t.Errorf("ParseLiteral(id1) = %v, %v", val, err)
	}
	val, err = IDScalar.ParseLiteral(&IntValue{Value: "123"})
	if err != nil || val != "123" {
		t.Errorf("ParseLiteral(123) = %v, %v", val, err)
	}
	_, err = IDScalar.ParseLiteral(&BooleanValue{Value: true})
	if err == nil {
		t.Error("expected error for bool literal as ID")
	}
}

func TestCoerceIntOverflow(t *testing.T) {
	_, err := coerceInt(int64(3000000000))
	if err == nil {
		t.Error("expected overflow error")
	}
	_, err = coerceInt(float64(3000000000))
	if err == nil {
		t.Error("expected overflow error")
	}
	_, err = coerceInt("not_a_number")
	if err == nil {
		t.Error("expected error for non-number string")
	}
	_, err = coerceFloat("not_a_number")
	if err == nil {
		t.Error("expected error for non-number string")
	}
}

func TestCoerceStringWithStruct(t *testing.T) {
	type Foo struct{ X int }
	val, _ := coerceString(Foo{X: 1})
	if val == nil {
		t.Error("expected non-nil result")
	}
}
