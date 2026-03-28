package json5

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// --- Test data generators ---

func makeSmallObject() []byte {
	return []byte(`{"name":"test","value":42,"active":true}`)
}

func makeMediumObject() []byte {
	// ~4KB flat object with varied value types.
	var b strings.Builder
	b.WriteString("{\n")
	for i := range 100 {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, `  "key_%03d": "value_%03d"`, i, i)
	}
	b.WriteString("\n}")
	return []byte(b.String())
}

func makeLargeObject() []byte {
	// ~100KB object.
	var b strings.Builder
	b.WriteString("{\n")
	for i := range 2000 {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, `  "key_%04d": "value %04d with some extra text to bulk it up a bit"`, i, i)
	}
	b.WriteString("\n}")
	return []byte(b.String())
}

func makeDeeplyNested(depth int) []byte {
	var b strings.Builder
	for range depth {
		b.WriteString(`{"a":`)
	}
	b.WriteString(`1`)
	for range depth {
		b.WriteByte('}')
	}
	return []byte(b.String())
}

func makeLargeString() []byte {
	// ~1MB single string value — worst case for fill() since the scanner
	// cannot compact mid-token.
	var b strings.Builder
	b.WriteByte('"')
	for range 1_000_000 {
		b.WriteByte('x')
	}
	b.WriteByte('"')
	return []byte(b.String())
}

func makeStringHeavyObject() []byte {
	// Object with many strings containing escape sequences.
	var b strings.Builder
	b.WriteString("{\n")
	for i := range 200 {
		if i > 0 {
			b.WriteString(",\n")
		}
		fmt.Fprintf(&b, `  "key_%03d": "line1\nline2\ttab\\slash\"quote\u0041"`, i)
	}
	b.WriteString("\n}")
	return []byte(b.String())
}

func makeStream(single []byte, count int) []byte {
	var b bytes.Buffer
	for i := range count {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(single)
	}
	return b.Bytes()
}

// --- Unmarshal benchmarks ---

func benchUnmarshal(b *testing.B, data []byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for range b.N {
		var v any
		if err := Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalSmall(b *testing.B)        { benchUnmarshal(b, makeSmallObject()) }
func BenchmarkUnmarshalMedium(b *testing.B)       { benchUnmarshal(b, makeMediumObject()) }
func BenchmarkUnmarshalLarge(b *testing.B)        { benchUnmarshal(b, makeLargeObject()) }
func BenchmarkUnmarshalDeeplyNested(b *testing.B) { benchUnmarshal(b, makeDeeplyNested(100)) }
func BenchmarkUnmarshalStringHeavy(b *testing.B)  { benchUnmarshal(b, makeStringHeavyObject()) }
func BenchmarkUnmarshalLargeString(b *testing.B)  { benchUnmarshal(b, makeLargeString()) }

// --- Decoder benchmarks (single value) ---

func benchDecoder(b *testing.B, data []byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for range b.N {
		dec := NewDecoder(bytes.NewReader(data))
		var v any
		if err := dec.Decode(&v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecoderSmall(b *testing.B)        { benchDecoder(b, makeSmallObject()) }
func BenchmarkDecoderMedium(b *testing.B)       { benchDecoder(b, makeMediumObject()) }
func BenchmarkDecoderLarge(b *testing.B)        { benchDecoder(b, makeLargeObject()) }
func BenchmarkDecoderDeeplyNested(b *testing.B) { benchDecoder(b, makeDeeplyNested(100)) }
func BenchmarkDecoderStringHeavy(b *testing.B)  { benchDecoder(b, makeStringHeavyObject()) }
func BenchmarkDecoderLargeString(b *testing.B)  { benchDecoder(b, makeLargeString()) }

// --- Decoder stream benchmark ---

func BenchmarkDecoderStream(b *testing.B) {
	single := makeSmallObject()
	stream := makeStream(single, 100)
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	for range b.N {
		dec := NewDecoder(bytes.NewReader(stream))
		for {
			var v any
			err := dec.Decode(&v)
			if err != nil {
				break
			}
		}
	}
}

// --- encoding/json Unmarshal benchmarks (baseline) ---

func benchStdUnmarshal(b *testing.B, data []byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for range b.N {
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStdUnmarshalSmall(b *testing.B)        { benchStdUnmarshal(b, makeSmallObject()) }
func BenchmarkStdUnmarshalMedium(b *testing.B)       { benchStdUnmarshal(b, makeMediumObject()) }
func BenchmarkStdUnmarshalLarge(b *testing.B)        { benchStdUnmarshal(b, makeLargeObject()) }
func BenchmarkStdUnmarshalDeeplyNested(b *testing.B) { benchStdUnmarshal(b, makeDeeplyNested(100)) }
func BenchmarkStdUnmarshalStringHeavy(b *testing.B)  { benchStdUnmarshal(b, makeStringHeavyObject()) }
func BenchmarkStdUnmarshalLargeString(b *testing.B)  { benchStdUnmarshal(b, makeLargeString()) }

// --- encoding/json Decoder benchmarks (baseline) ---

func benchStdDecoder(b *testing.B, data []byte) {
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for range b.N {
		dec := json.NewDecoder(bytes.NewReader(data))
		var v any
		if err := dec.Decode(&v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStdDecoderSmall(b *testing.B)        { benchStdDecoder(b, makeSmallObject()) }
func BenchmarkStdDecoderMedium(b *testing.B)       { benchStdDecoder(b, makeMediumObject()) }
func BenchmarkStdDecoderLarge(b *testing.B)        { benchStdDecoder(b, makeLargeObject()) }
func BenchmarkStdDecoderDeeplyNested(b *testing.B) { benchStdDecoder(b, makeDeeplyNested(100)) }
func BenchmarkStdDecoderStringHeavy(b *testing.B)  { benchStdDecoder(b, makeStringHeavyObject()) }
func BenchmarkStdDecoderLargeString(b *testing.B)  { benchStdDecoder(b, makeLargeString()) }

func BenchmarkStdDecoderStream(b *testing.B) {
	single := makeSmallObject()
	stream := makeStream(single, 100)
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	for range b.N {
		dec := json.NewDecoder(bytes.NewReader(stream))
		for {
			var v any
			err := dec.Decode(&v)
			if err != nil {
				break
			}
		}
	}
}
