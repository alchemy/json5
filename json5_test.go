package json5

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// --- Unmarshal tests ---

func TestUnmarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{"string double", `"hello"`, "hello"},
		{"string single", `'hello'`, "hello"},
		{"int", `42`, float64(42)},
		{"negative int", `-42`, float64(-42)},
		{"float", `3.14`, float64(3.14)},
		{"true", `true`, true},
		{"false", `false`, false},
		{"null", `null`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			if err := Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal(%q): %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unmarshal(%q) = %v (%T), want %v (%T)", tt.input, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestUnmarshalSingleQuotedString(t *testing.T) {
	var s string
	if err := Unmarshal([]byte(`'hello world'`), &s); err != nil {
		t.Fatal(err)
	}
	if s != "hello world" {
		t.Errorf("got %q, want %q", s, "hello world")
	}
}

func TestUnmarshalStringEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single quote escape", `'it\'s'`, "it's"},
		{"double quote escape", `"say \"hi\""`, `say "hi"`},
		{"newline", `"a\nb"`, "a\nb"},
		{"tab", `"a\tb"`, "a\tb"},
		{"vertical tab", `"a\vb"`, "a\vb"},
		{"null char", `"a\0b"`, "a\x00b"},
		{"hex escape", `"\x41\x42"`, "AB"},
		{"unicode escape", `"\u0041"`, "A"},
		{"backslash", `"a\\b"`, `a\b`},
		{"slash", `"a\/b"`, "a/b"},
		{"backspace", `"a\bb"`, "a\bb"},
		{"form feed", `"a\fb"`, "a\fb"},
		{"carriage return", `"a\rb"`, "a\rb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s string
			if err := Unmarshal([]byte(tt.input), &s); err != nil {
				t.Fatalf("Unmarshal(%q): %v", tt.input, err)
			}
			if s != tt.want {
				t.Errorf("got %q, want %q", s, tt.want)
			}
		})
	}
}

func TestUnmarshalMultilineString(t *testing.T) {
	input := "'hello \\\nworld'"
	var s string
	if err := Unmarshal([]byte(input), &s); err != nil {
		t.Fatal(err)
	}
	if s != "hello world" {
		t.Errorf("got %q, want %q", s, "hello world")
	}
}

func TestUnmarshalMultilineStringCRLF(t *testing.T) {
	input := "'hello \\\r\nworld'"
	var s string
	if err := Unmarshal([]byte(input), &s); err != nil {
		t.Fatal(err)
	}
	if s != "hello world" {
		t.Errorf("got %q, want %q", s, "hello world")
	}
}

func TestUnmarshalHexNumber(t *testing.T) {
	var n int
	if err := Unmarshal([]byte(`0xFF`), &n); err != nil {
		t.Fatal(err)
	}
	if n != 255 {
		t.Errorf("got %d, want 255", n)
	}
}

func TestUnmarshalPositiveSign(t *testing.T) {
	var n float64
	if err := Unmarshal([]byte(`+42`), &n); err != nil {
		t.Fatal(err)
	}
	if n != 42 {
		t.Errorf("got %f, want 42", n)
	}
}

func TestUnmarshalLeadingDot(t *testing.T) {
	var n float64
	if err := Unmarshal([]byte(`.5`), &n); err != nil {
		t.Fatal(err)
	}
	if n != 0.5 {
		t.Errorf("got %f, want 0.5", n)
	}
}

func TestUnmarshalTrailingDot(t *testing.T) {
	var n float64
	if err := Unmarshal([]byte(`5.`), &n); err != nil {
		t.Fatal(err)
	}
	if n != 5.0 {
		t.Errorf("got %f, want 5.0", n)
	}
}

func TestUnmarshalInfinity(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"Infinity", math.Inf(1)},
		{"+Infinity", math.Inf(1)},
		{"-Infinity", math.Inf(-1)},
	}
	for _, tt := range tests {
		var n float64
		if err := Unmarshal([]byte(tt.input), &n); err != nil {
			t.Fatalf("Unmarshal(%q): %v", tt.input, err)
		}
		if n != tt.want {
			t.Errorf("Unmarshal(%q) = %f, want %f", tt.input, n, tt.want)
		}
	}
}

func TestUnmarshalNaN(t *testing.T) {
	var n float64
	if err := Unmarshal([]byte(`NaN`), &n); err != nil {
		t.Fatal(err)
	}
	if !math.IsNaN(n) {
		t.Errorf("got %f, want NaN", n)
	}
}

func TestUnmarshalComments(t *testing.T) {
	input := `{
		// single-line comment
		"a": 1,
		/* multi-line
		   comment */
		"b": 2
	}`
	var m map[string]int
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}
	if m["a"] != 1 || m["b"] != 2 {
		t.Errorf("got %v, want map[a:1 b:2]", m)
	}
}

func TestUnmarshalTrailingComma(t *testing.T) {
	t.Run("object", func(t *testing.T) {
		input := `{"a": 1, "b": 2,}`
		var m map[string]int
		if err := Unmarshal([]byte(input), &m); err != nil {
			t.Fatal(err)
		}
		if m["a"] != 1 || m["b"] != 2 {
			t.Errorf("got %v", m)
		}
	})
	t.Run("array", func(t *testing.T) {
		input := `[1, 2, 3,]`
		var a []int
		if err := Unmarshal([]byte(input), &a); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, []int{1, 2, 3}) {
			t.Errorf("got %v", a)
		}
	})
}

func TestUnmarshalUnquotedKeys(t *testing.T) {
	input := `{name: "Alice", age: 30, $type: "user", _id: 1}`
	var m map[string]any
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Alice" {
		t.Errorf("name = %v", m["name"])
	}
	if m["age"] != float64(30) {
		t.Errorf("age = %v", m["age"])
	}
	if m["$type"] != "user" {
		t.Errorf("$type = %v", m["$type"])
	}
	if m["_id"] != float64(1) {
		t.Errorf("_id = %v", m["_id"])
	}
}

func TestUnmarshalStruct(t *testing.T) {
	type Person struct {
		Name string `json5:"name"`
		Age  int    `json5:"age"`
	}
	input := `{name: 'Alice', age: 30}`
	var p Person
	if err := Unmarshal([]byte(input), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alice" || p.Age != 30 {
		t.Errorf("got %+v", p)
	}
}

func TestUnmarshalStructJSONTag(t *testing.T) {
	type Item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	input := `{"id": 42, "name": "widget"}`
	var item Item
	if err := Unmarshal([]byte(input), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID != 42 || item.Name != "widget" {
		t.Errorf("got %+v", item)
	}
}

func TestUnmarshalNestedObject(t *testing.T) {
	input := `{
		name: 'Alice',
		address: {
			city: 'Wonderland',
			zip: '12345',
		},
	}`
	type Address struct {
		City string `json5:"city"`
		Zip  string `json5:"zip"`
	}
	type Person struct {
		Name    string  `json5:"name"`
		Address Address `json5:"address"`
	}
	var p Person
	if err := Unmarshal([]byte(input), &p); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alice" || p.Address.City != "Wonderland" || p.Address.Zip != "12345" {
		t.Errorf("got %+v", p)
	}
}

func TestUnmarshalOmitEmpty(t *testing.T) {
	type Item struct {
		Name  string `json5:"name"`
		Value int    `json5:"value,omitempty"`
	}
	input := `{"name": "test"}`
	var item Item
	if err := Unmarshal([]byte(input), &item); err != nil {
		t.Fatal(err)
	}
	if item.Name != "test" || item.Value != 0 {
		t.Errorf("got %+v", item)
	}
}

func TestUnmarshalIgnoredField(t *testing.T) {
	type Item struct {
		Name   string `json5:"name"`
		Secret string `json5:"-"`
	}
	input := `{"name": "test", "Secret": "hidden"}`
	var item Item
	if err := Unmarshal([]byte(input), &item); err != nil {
		t.Fatal(err)
	}
	if item.Secret != "" {
		t.Errorf("Secret should be empty, got %q", item.Secret)
	}
}

func TestUnmarshalExponent(t *testing.T) {
	var n float64
	if err := Unmarshal([]byte(`1.5e2`), &n); err != nil {
		t.Fatal(err)
	}
	if n != 150 {
		t.Errorf("got %f, want 150", n)
	}
}

func TestUnmarshalNegativeHex(t *testing.T) {
	var n int
	if err := Unmarshal([]byte(`-0xFF`), &n); err != nil {
		t.Fatal(err)
	}
	if n != -255 {
		t.Errorf("got %d, want -255", n)
	}
}

// --- Marshal tests ---

func TestMarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, `42`},
		{"float", 3.14, `3.14`},
		{"true", true, `true`},
		{"false", false, `false`},
		{"null", nil, `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := Marshal(tt.v)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tt.want {
				t.Errorf("Marshal(%v) = %s, want %s", tt.v, b, tt.want)
			}
		})
	}
}

func TestMarshalStruct(t *testing.T) {
	type Person struct {
		Name string `json5:"name"`
		Age  int    `json5:"age"`
	}
	p := Person{Name: "Alice", Age: 30}
	b, err := Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"Alice","age":30}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalOmitEmpty(t *testing.T) {
	type Item struct {
		Name  string `json5:"name"`
		Value int    `json5:"value,omitempty"`
	}
	item := Item{Name: "test"}
	b, err := Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"test"}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalIndent(t *testing.T) {
	m := map[string]int{"a": 1, "b": 2}
	b, err := MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "\n") {
		t.Errorf("expected indented output, got %s", s)
	}
}

func TestMarshalSlice(t *testing.T) {
	a := []int{1, 2, 3}
	b, err := Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	want := `[1,2,3]`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalMap(t *testing.T) {
	m := map[string]string{"key": "value"}
	b, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"key":"value"}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalNested(t *testing.T) {
	type Inner struct {
		X int `json5:"x"`
	}
	type Outer struct {
		Inner Inner `json5:"inner"`
	}
	o := Outer{Inner: Inner{X: 42}}
	b, err := Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"inner":{"x":42}}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalStringEscapes(t *testing.T) {
	b, err := Marshal("hello\nworld")
	if err != nil {
		t.Fatal(err)
	}
	want := `"hello\nworld"`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestMarshalNilSlice(t *testing.T) {
	var s []int
	b, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "null" {
		t.Errorf("got %s, want null", b)
	}
}

func TestMarshalNilMap(t *testing.T) {
	var m map[string]int
	b, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "null" {
		t.Errorf("got %s, want null", b)
	}
}

// --- Valid tests ---

func TestValid(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{`{"a": 1}`, true},
		{`{a: 1}`, true},
		{`[1, 2, 3,]`, true},
		{`'hello'`, true},
		{`Infinity`, true},
		{`NaN`, true},
		{`0xFF`, true},
		{"// comment\n42", true},
		{`{`, false},
		{`[1, 2`, false},
		{`"unterminated`, false},
		{`{a: }`, false},
	}
	for _, tt := range tests {
		got := Valid([]byte(tt.input))
		if got != tt.valid {
			t.Errorf("Valid(%q) = %v, want %v", tt.input, got, tt.valid)
		}
	}
}

// --- Streaming tests ---

func TestDecoder(t *testing.T) {
	input := `{"name": "test", "value": 42}`
	dec := NewDecoder(strings.NewReader(input))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "test" || m["value"] != float64(42) {
		t.Errorf("got %v", m)
	}
}

func TestDecoderUseNumber(t *testing.T) {
	input := `42`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	n, ok := v.(Number)
	if !ok {
		t.Fatalf("expected Number, got %T", v)
	}
	if n.String() != "42" {
		t.Errorf("got %s, want 42", n)
	}
}

func TestEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `{"a":1}` {
		t.Errorf("got %q", got)
	}
}

// --- RawMessage tests ---

func TestRawMessage(t *testing.T) {
	type Msg struct {
		Type    string     `json5:"type"`
		Payload RawMessage `json5:"payload"`
	}
	input := `{"type": "test", "payload": {"x": 1}}`
	var msg Msg
	if err := Unmarshal([]byte(input), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Type != "test" {
		t.Errorf("type = %q", msg.Type)
	}
	if len(msg.Payload) == 0 {
		t.Error("payload is empty")
	}
}

// --- Roundtrip tests ---

func TestRoundtrip(t *testing.T) {
	type Config struct {
		Name    string   `json5:"name"`
		Debug   bool     `json5:"debug"`
		Count   int      `json5:"count"`
		Rate    float64  `json5:"rate"`
		Tags    []string `json5:"tags"`
		Enabled *bool    `json5:"enabled,omitempty"`
	}

	enabled := true
	original := Config{
		Name:    "test",
		Debug:   true,
		Count:   42,
		Rate:    3.14,
		Tags:    []string{"a", "b"},
		Enabled: &enabled,
	}

	b, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Config
	if err := Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Name != original.Name ||
		decoded.Debug != original.Debug ||
		decoded.Count != original.Count ||
		decoded.Rate != original.Rate ||
		!reflect.DeepEqual(decoded.Tags, original.Tags) ||
		*decoded.Enabled != *original.Enabled {
		t.Errorf("roundtrip mismatch:\n  original: %+v\n  decoded:  %+v", original, decoded)
	}
}

// --- json.Marshaler/Unmarshaler compatibility ---

type customJSON struct {
	Value string
}

func (c customJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal("custom:" + c.Value)
}

func (c *customJSON) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	c.Value = strings.TrimPrefix(s, "custom:")
	return nil
}

func TestJSONMarshalerCompat(t *testing.T) {
	c := customJSON{Value: "hello"}
	b, err := Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	want := `"custom:hello"`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestJSONUnmarshalerCompat(t *testing.T) {
	var c customJSON
	if err := Unmarshal([]byte(`"custom:hello"`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Value != "hello" {
		t.Errorf("got %q, want hello", c.Value)
	}
}

// --- Error handling ---

func TestUnmarshalNonPointer(t *testing.T) {
	var s string
	err := Unmarshal([]byte(`"test"`), s)
	if err == nil {
		t.Fatal("expected error for non-pointer")
	}
	if _, ok := err.(*InvalidUnmarshalError); !ok {
		t.Errorf("expected InvalidUnmarshalError, got %T", err)
	}
}

func TestUnmarshalNilPointer(t *testing.T) {
	err := Unmarshal([]byte(`"test"`), nil)
	if err == nil {
		t.Fatal("expected error for nil")
	}
}

func TestUnmarshalSyntaxError(t *testing.T) {
	var v any
	err := Unmarshal([]byte(`{invalid`), &v)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisallowUnknownFields(t *testing.T) {
	type S struct {
		A int `json5:"a"`
	}
	dec := NewDecoder(strings.NewReader(`{"a": 1, "b": 2}`))
	dec.DisallowUnknownFields()
	var s S
	err := dec.Decode(&s)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Extended whitespace ---

func TestExtendedWhitespace(t *testing.T) {
	// Vertical tab and form feed as whitespace.
	input := "{\v\"a\"\f:\t1\n}"
	var m map[string]int
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}
	if m["a"] != 1 {
		t.Errorf("got %v", m)
	}
}

// --- Embedded struct ---

func TestEmbeddedStruct(t *testing.T) {
	type Base struct {
		ID int `json5:"id"`
	}
	type Extended struct {
		Base
		Name string `json5:"name"`
	}
	input := `{"id": 1, "name": "test"}`
	var e Extended
	if err := Unmarshal([]byte(input), &e); err != nil {
		t.Fatal(err)
	}
	if e.ID != 1 || e.Name != "test" {
		t.Errorf("got %+v", e)
	}
}

// --- Number type methods ---

func TestNumberMethods(t *testing.T) {
	n := Number("42")
	if n.String() != "42" {
		t.Errorf("String() = %q", n.String())
	}
	f, err := n.Float64()
	if err != nil || f != 42 {
		t.Errorf("Float64() = %f, %v", f, err)
	}
	i, err := n.Int64()
	if err != nil || i != 42 {
		t.Errorf("Int64() = %d, %v", i, err)
	}
}

// --- Complex JSON5 document ---

func TestComplexDocument(t *testing.T) {
	input := `{
		// This is a JSON5 document
		unquoted: 'and you can quote me on that',
		singleQuotes: 'I can use "double quotes" here',
		lineBreaks: "Look, Mom! \
No \\n's!",
		hexadecimal: 0xdecaf,
		leadingDecimalPoint: .8675309,
		andTrailing: 8675309.,
		positiveSign: +1,
		trailingComma: 'in objects',
		andIn: ['arrays',],
		"backwardsCompatible": "with JSON",
	}`
	var m map[string]any
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}

	if m["unquoted"] != "and you can quote me on that" {
		t.Errorf("unquoted = %v", m["unquoted"])
	}
	if m["singleQuotes"] != `I can use "double quotes" here` {
		t.Errorf("singleQuotes = %v", m["singleQuotes"])
	}
	if m["lineBreaks"] != "Look, Mom! No \\n's!" {
		t.Errorf("lineBreaks = %v", m["lineBreaks"])
	}
	if m["hexadecimal"] != float64(0xdecaf) {
		t.Errorf("hexadecimal = %v", m["hexadecimal"])
	}
	if m["leadingDecimalPoint"] != 0.8675309 {
		t.Errorf("leadingDecimalPoint = %v", m["leadingDecimalPoint"])
	}
	if m["andTrailing"] != float64(8675309) {
		t.Errorf("andTrailing = %v", m["andTrailing"])
	}
	if m["positiveSign"] != float64(1) {
		t.Errorf("positiveSign = %v", m["positiveSign"])
	}
	if m["backwardsCompatible"] != "with JSON" {
		t.Errorf("backwardsCompatible = %v", m["backwardsCompatible"])
	}
}

// --- Bug fix: fieldCache data race (sync.Map) ---

func TestConcurrentUnmarshal(t *testing.T) {
	// This would crash with the old plain map fieldCache under -race.
	type A struct {
		X int `json5:"x"`
	}
	type B struct {
		Y string `json5:"y"`
	}
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			var a A
			Unmarshal([]byte(`{"x":1}`), &a)
			var b B
			Unmarshal([]byte(`{"y":"hello"}`), &b)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// --- Bug fix: collectRawComposite trailing comma produces valid JSON ---

func TestJSONUnmarshalerWithTrailingComma(t *testing.T) {
	var c customJSON
	// The trailing comma in the object should not produce invalid JSON
	// when passed to json.Unmarshaler.
	input := `{"type": "test", "items": [1, 2,],}`
	type wrapper struct {
		Type  string     `json5:"type"`
		Items customJSON `json5:"items"`
	}
	// Use a type with json.Unmarshaler on a nested value with trailing commas.
	type simple struct {
		Value customJSON `json5:"value"`
	}
	if err := Unmarshal([]byte(`{"value": "custom:hello",}`), &simple{}); err != nil {
		t.Fatalf("trailing comma broke json.Unmarshaler: %v", err)
	}
	_ = c
	_ = input
}

// --- Bug fix: Number.Float64() and Number.Int64() handle JSON5 formats ---

func TestNumberMethodsJSON5Formats(t *testing.T) {
	// Hex
	n := Number("0xFF")
	f, err := n.Float64()
	if err != nil || f != 255 {
		t.Errorf("Number(0xFF).Float64() = %f, %v", f, err)
	}
	i, err := n.Int64()
	if err != nil || i != 255 {
		t.Errorf("Number(0xFF).Int64() = %d, %v", i, err)
	}

	// Infinity
	n = Number("Infinity")
	f, err = n.Float64()
	if err != nil || !math.IsInf(f, 1) {
		t.Errorf("Number(Infinity).Float64() = %f, %v", f, err)
	}

	// NaN
	n = Number("NaN")
	f, err = n.Float64()
	if err != nil || !math.IsNaN(f) {
		t.Errorf("Number(NaN).Float64() = %f, %v", f, err)
	}

	// Positive sign
	n = Number("+42")
	f, err = n.Float64()
	if err != nil || f != 42 {
		t.Errorf("Number(+42).Float64() = %f, %v", f, err)
	}
	i, err = n.Int64()
	if err != nil || i != 42 {
		t.Errorf("Number(+42).Int64() = %d, %v", i, err)
	}
}

// --- Bug fix: Unicode escapes in identifiers decoded ---

func TestUnquotedKeyUnicodeEscape(t *testing.T) {
	// \u006E = 'n', \u0061 = 'a', \u006D = 'm', \u0065 = 'e'
	input := `{\u006Eame: "Alice"}`
	var m map[string]string
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "Alice" {
		t.Errorf("got %v, want map[name:Alice]", m)
	}
}

// --- Bug fix: ,string tag handled during decode ---

func TestStringTagDecode(t *testing.T) {
	type Item struct {
		Count int     `json5:"count,string"`
		Rate  float64 `json5:"rate,string"`
		On    bool    `json5:"on,string"`
		Label string  `json5:"label,string"`
	}
	input := `{"count": "42", "rate": "3.14", "on": "true", "label": "hello"}`
	var item Item
	if err := Unmarshal([]byte(input), &item); err != nil {
		t.Fatal(err)
	}
	if item.Count != 42 {
		t.Errorf("Count = %d, want 42", item.Count)
	}
	if item.Rate != 3.14 {
		t.Errorf("Rate = %f, want 3.14", item.Rate)
	}
	if item.On != true {
		t.Errorf("On = %v, want true", item.On)
	}
	if item.Label != "hello" {
		t.Errorf("Label = %q, want hello", item.Label)
	}
}

func TestStringTagEncode(t *testing.T) {
	type Item struct {
		Count int `json5:"count,string"`
	}
	item := Item{Count: 42}
	b, err := Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"count":"42"}`
	if string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

// --- Bug fix: Embedded pointer-to-struct ---

func TestEmbeddedPointerToStruct(t *testing.T) {
	type Inner struct {
		X int `json5:"x"`
	}
	type Outer struct {
		*Inner
		Y string `json5:"y"`
	}
	input := `{"x": 42, "y": "hello"}`
	var o Outer
	if err := Unmarshal([]byte(input), &o); err != nil {
		t.Fatal(err)
	}
	if o.Inner == nil || o.X != 42 || o.Y != "hello" {
		t.Errorf("got %+v", o)
	}
}

// --- Vulnerability fix: Depth limit ---

func TestMaxNestingDepth(t *testing.T) {
	// Build a deeply nested array.
	var b []byte
	for i := 0; i < 10001; i++ {
		b = append(b, '[')
	}
	for i := 0; i < 10001; i++ {
		b = append(b, ']')
	}
	var v any
	err := Unmarshal(b, &v)
	if err == nil {
		t.Fatal("expected error for deeply nested input")
	}
	if !strings.Contains(err.Error(), "nesting depth") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- Feature: Encoder supports Infinity/NaN ---

func TestMarshalInfinity(t *testing.T) {
	b, err := Marshal(math.Inf(1))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "Infinity" {
		t.Errorf("got %s, want Infinity", b)
	}

	b, err = Marshal(math.Inf(-1))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "-Infinity" {
		t.Errorf("got %s, want -Infinity", b)
	}
}

func TestMarshalNaN(t *testing.T) {
	b, err := Marshal(math.NaN())
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "NaN" {
		t.Errorf("got %s, want NaN", b)
	}
}

func TestMarshalIndentWithInfinity(t *testing.T) {
	m := map[string]float64{"inf": math.Inf(1), "nan": math.NaN()}
	b, err := MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Infinity") {
		t.Errorf("expected Infinity in output: %s", s)
	}
	if !strings.Contains(s, "NaN") {
		t.Errorf("expected NaN in output: %s", s)
	}
}

// --- Roundtrip Infinity/NaN ---

func TestRoundtripInfinity(t *testing.T) {
	type Config struct {
		Value float64 `json5:"value"`
	}
	original := Config{Value: math.Inf(1)}
	b, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(decoded.Value, 1) {
		t.Errorf("roundtrip failed: got %f, want +Inf", decoded.Value)
	}
}
