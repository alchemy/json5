package json5

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestOrderedMapBasicOperations(t *testing.T) {
	m := NewOrderedMap()
	m.Set("z", 1)
	m.Set("a", 2)
	m.Set("m", 3)

	if m.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", m.Len())
	}

	// Keys in insertion order.
	keys := m.Keys()
	want := []string{"z", "a", "m"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("Keys() = %v, want %v", keys, want)
	}

	// Get.
	v, ok := m.Get("a")
	if !ok || v != 2 {
		t.Errorf("Get(a) = %v, %v; want 2, true", v, ok)
	}
	_, ok = m.Get("missing")
	if ok {
		t.Error("Get(missing) returned ok=true")
	}

	// Update preserves order.
	m.Set("z", 99)
	keys = m.Keys()
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("after update, Keys() = %v, want %v", keys, want)
	}
	v, _ = m.Get("z")
	if v != 99 {
		t.Errorf("after update, Get(z) = %v, want 99", v)
	}

	// Delete.
	m.Delete("a")
	keys = m.Keys()
	wantAfterDelete := []string{"z", "m"}
	if !reflect.DeepEqual(keys, wantAfterDelete) {
		t.Errorf("after delete, Keys() = %v, want %v", keys, wantAfterDelete)
	}
}

func TestUnmarshalOrderedMap(t *testing.T) {
	input := `{z: 1, a: 2, m: 3}`
	var m OrderedMap
	if err := Unmarshal([]byte(input), &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	keys := m.Keys()
	want := []string{"z", "a", "m"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("Keys() = %v, want %v", keys, want)
	}
	v, _ := m.Get("a")
	if v != float64(2) {
		t.Errorf("Get(a) = %v, want 2", v)
	}
}

func TestDecoderUseOrderedMap(t *testing.T) {
	input := `{z: 1, a: 2, m: 3}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	m, ok := got.(*OrderedMap)
	if !ok {
		t.Fatalf("got %T, want *OrderedMap", got)
	}

	keys := m.Keys()
	want := []string{"z", "a", "m"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("Keys() = %v, want %v", keys, want)
	}
}

func TestDecoderUseOrderedMapNested(t *testing.T) {
	input := `{outer: {b: 2, a: 1}, list: [1, {d: 4, c: 3}]}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	root, ok := got.(*OrderedMap)
	if !ok {
		t.Fatalf("got %T, want *OrderedMap", got)
	}

	// Check outer is also an OrderedMap.
	outerVal, _ := root.Get("outer")
	outer, ok := outerVal.(*OrderedMap)
	if !ok {
		t.Fatalf("outer is %T, want *OrderedMap", outerVal)
	}
	if keys := outer.Keys(); !reflect.DeepEqual(keys, []string{"b", "a"}) {
		t.Errorf("outer keys = %v, want [b a]", keys)
	}

	// Check nested object inside array.
	listVal, _ := root.Get("list")
	list, ok := listVal.([]any)
	if !ok {
		t.Fatalf("list is %T, want []any", listVal)
	}
	nested, ok := list[1].(*OrderedMap)
	if !ok {
		t.Fatalf("list[1] is %T, want *OrderedMap", list[1])
	}
	if keys := nested.Keys(); !reflect.DeepEqual(keys, []string{"d", "c"}) {
		t.Errorf("nested keys = %v, want [d c]", keys)
	}
}

func TestMarshalOrderedMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("z", float64(1))
	m.Set("a", float64(2))
	m.Set("m", float64(3))

	b, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	want := `{"z":1,"a":2,"m":3}`
	if got != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}
}

func TestOrderedMapRoundTrip(t *testing.T) {
	input := `{"z":1,"a":2,"m":3}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	b, err := Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != input {
		t.Errorf("round-trip: got %s, want %s", b, input)
	}
}

func TestDecoderWithoutUseOrderedMap(t *testing.T) {
	// Without UseOrderedMap, objects should still decode to map[string]any.
	input := `{"a": 1}`
	dec := NewDecoder(strings.NewReader(input))

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if _, ok := got.(map[string]any); !ok {
		t.Fatalf("got %T, want map[string]any", got)
	}
}

func TestDecoderUseOrderedMapWithUseNumber(t *testing.T) {
	input := `{a: 42}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()
	dec.UseNumber()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	m, ok := got.(*OrderedMap)
	if !ok {
		t.Fatalf("got %T, want *OrderedMap", got)
	}
	v, _ := m.Get("a")
	n, ok := v.(Number)
	if !ok {
		t.Fatalf("value is %T, want Number", v)
	}
	if n.String() != "42" {
		t.Errorf("number = %q, want %q", n, "42")
	}
}

func TestDecoderUseOrderedMapStream(t *testing.T) {
	input := `{"b":1,"a":2} {"d":3,"c":4}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var results []*OrderedMap
	for {
		var v any
		err := dec.Decode(&v)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		m, ok := v.(*OrderedMap)
		if !ok {
			t.Fatalf("got %T, want *OrderedMap", v)
		}
		results = append(results, m)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if keys := results[0].Keys(); !reflect.DeepEqual(keys, []string{"b", "a"}) {
		t.Errorf("first object keys = %v, want [b a]", keys)
	}
	if keys := results[1].Keys(); !reflect.DeepEqual(keys, []string{"d", "c"}) {
		t.Errorf("second object keys = %v, want [d c]", keys)
	}
}

func TestMarshalIndentOrderedMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("z", float64(1))
	m.Set("a", float64(2))

	b, err := MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	want := "{\n  \"z\": 1,\n  \"a\": 2\n}"
	if string(b) != want {
		t.Errorf("MarshalIndent =\n%s\nwant:\n%s", b, want)
	}
}

func TestOrderedMapMarshalNestedOrderedMap(t *testing.T) {
	inner := NewOrderedMap()
	inner.Set("y", true)
	inner.Set("x", false)

	outer := NewOrderedMap()
	outer.Set("inner", inner)
	outer.Set("val", "hello")

	b, err := Marshal(outer)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{"inner":{"y":true,"x":false},"val":"hello"}`
	if string(b) != want {
		t.Errorf("Marshal = %s, want %s", b, want)
	}
}

func TestEncoderOrderedMap(t *testing.T) {
	m := NewOrderedMap()
	m.Set("z", float64(1))
	m.Set("a", float64(2))

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	want := `{"z":1,"a":2}`
	if got != want {
		t.Errorf("Encode = %s, want %s", got, want)
	}
}

func TestOrderedMapEmptyObject(t *testing.T) {
	input := `{}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	m, ok := got.(*OrderedMap)
	if !ok {
		t.Fatalf("got %T, want *OrderedMap", got)
	}
	if m.Len() != 0 {
		t.Errorf("Len() = %d, want 0", m.Len())
	}

	b, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Errorf("Marshal = %s, want {}", b)
	}
}

func TestOrderedMapTrailingComma(t *testing.T) {
	input := `{z: 1, a: 2,}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	m := got.(*OrderedMap)
	if keys := m.Keys(); !reflect.DeepEqual(keys, []string{"z", "a"}) {
		t.Errorf("Keys() = %v, want [z a]", keys)
	}
}

func TestOrderedMapComments(t *testing.T) {
	input := `{
		// leading comment
		z: 1,
		/* block comment */ a: 2
	}`
	dec := NewDecoder(strings.NewReader(input))
	dec.UseOrderedMap()

	var got any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	m := got.(*OrderedMap)
	if keys := m.Keys(); !reflect.DeepEqual(keys, []string{"z", "a"}) {
		t.Errorf("Keys() = %v, want [z a]", keys)
	}
}
