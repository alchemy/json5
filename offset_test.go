package json5

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// rawCollector implements json.Unmarshaler so decoding goes through collectRaw.
type rawCollector struct{}

func (*rawCollector) UnmarshalJSON([]byte) error { return nil }

type offsetStruct struct {
	A int `json5:"a"`
	B int `json5:"b"`
	C any `json5:"c"`
}

type offsetFontStruct struct {
	Body struct {
		FontSize   float64 `json5:"fontSize"`
		LineHeight float64 `json5:"lineHeight"`
	} `json5:"body"`
}

// offsetCases lists inputs whose syntax error is about an already-scanned
// token. at is a substring of input whose first occurrence starts at the
// offending token; the error offset must equal its index.
var offsetCases = []struct {
	name   string
	input  string
	at     string
	target func() any
}{
	// Objects: struct, map, interface, ordered map, skipped, raw-collected.
	{"struct/comma", "{a: 1\n  b: 2}", "b: 2", func() any { return new(offsetStruct) }},
	{"struct/colon", "{a 1}", "1}", func() any { return new(offsetStruct) }},
	{"struct/key", "{a: 1, [: 2}", "[:", func() any { return new(offsetStruct) }},
	{"map/comma", "{a: 1\n  b: 2}", "b: 2", func() any { return new(map[string]int) }},
	{"map/colon", "{a 1}", "1}", func() any { return new(map[string]int) }},
	{"any/comma", "{a: 1\n  b: 2}", "b: 2", func() any { return new(any) }},
	{"any/colon", "{a 1}", "1}", func() any { return new(any) }},
	{"ordered/comma", "{a: 1\n  b: 2}", "b: 2", func() any { return new(OrderedMap) }},
	{"ordered/colon", "{a 1}", "1}", func() any { return new(OrderedMap) }},
	{"skip/comma", "{x: {p: 1\n  q: 2}, a: 1}", "q: 2", func() any { return new(offsetStruct) }},
	{"skip/colon", "{x: {p 1}, a: 1}", "1}", func() any { return new(offsetStruct) }},
	{"skip/array-comma", "{x: [1\n  2], a: 1}", "2]", func() any { return new(offsetStruct) }},
	{"skip/value", "{x: :, a: 1}", ":, a", func() any { return new(offsetStruct) }},
	{"raw/comma", "{p: 1\n  q: 2}", "q: 2", func() any { return new(rawCollector) }},
	{"raw/colon", "{p 1}", "1}", func() any { return new(rawCollector) }},
	{"raw/array-comma", "[1\n  2]", "2]", func() any { return new(rawCollector) }},
	{"raw/value", "]", "]", func() any { return new(rawCollector) }},
	{"raw/nested-value", "{p: ]}", "]}", func() any { return new(rawCollector) }},
	{"raw/array-value", "[1, :]", ":]", func() any { return new(rawCollector) }},
	{"raw/leading-comma", "[,1]", ",1]", func() any { return new(rawCollector) }},
	{"raw/identifier", "{p: foo}", "foo}", func() any { return new(rawCollector) }},
	{"raw/eof", "{p: ", "", func() any { return new(rawCollector) }},
	// Arrays: slice, fixed array, interface.
	{"slice/comma", "[1\n  2]", "2]", func() any { return new([]int) }},
	{"fixed/comma", "[1\n  2]", "2]", func() any { return new([2]int) }},
	{"anyarray/comma", "[1\n  2]", "2]", func() any { return new(any) }},
	// Unexpected value tokens.
	{"struct/value", "{a: :}", ":}", func() any { return new(offsetStruct) }},
	{"any/value", "{c: ]}", "]}", func() any { return new(any) }},
	{"struct/eof", "{c: ", "", func() any { return new(offsetStruct) }},
	{"any/eof", "{c: ", "", func() any { return new(any) }},
	// Nested inside a struct's interface field.
	{"struct-any/comma", "{c: {p: 1\n  q: 2}}", "q: 2", func() any { return new(offsetStruct) }},
	// The issue's repro.
	{"repro", "{\n  body: {\n    fontSize: 11\n    lineHeight: 1.5,\n  },\n}\n", "lineHeight", func() any { return new(offsetFontStruct) }},
}

func expectedOffset(t *testing.T, input, at string) int64 {
	t.Helper()
	if at == "" {
		return int64(len(input))
	}
	i := strings.Index(input, at)
	if i < 0 {
		t.Fatalf("marker %q not in input", at)
	}
	return int64(i)
}

func checkOffset(t *testing.T, err error, want int64) {
	t.Helper()
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("got error %v (%T), want *SyntaxError", err, err)
	}
	if se.Offset != want {
		t.Errorf("%v: offset = %d, want %d", err, se.Offset, want)
	}
}

func TestSyntaxErrorOffsetUnmarshal(t *testing.T) {
	for _, tc := range offsetCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Unmarshal([]byte(tc.input), tc.target())
			checkOffset(t, err, expectedOffset(t, tc.input, tc.at))
		})
	}
}

func TestSyntaxErrorOffsetDecoder(t *testing.T) {
	for _, tc := range offsetCases {
		t.Run(tc.name, func(t *testing.T) {
			want := expectedOffset(t, tc.input, tc.at)
			err := NewDecoder(strings.NewReader(tc.input)).Decode(tc.target())
			checkOffset(t, err, want)

			// One byte at a time: the scanner refills on every byte.
			err = NewDecoder(iotest.OneByteReader(strings.NewReader(tc.input))).Decode(tc.target())
			checkOffset(t, err, want)
		})
	}
}

// TestSyntaxErrorOffsetAfterRefill puts the error well past the scanner's
// 4096-byte read size, after earlier values in the stream have been decoded
// and discarded, so the buffer has been compacted (s.base > 0) and refilled.
func TestSyntaxErrorOffsetAfterRefill(t *testing.T) {
	filler := `{a: 1, b: "` + strings.Repeat("x", 500) + `"} // filler` + "\n"
	const nFiller = 25 // ~13KB, several 4096-byte reads
	prefix := strings.Repeat(filler, nFiller)

	for _, tc := range offsetCases {
		t.Run(tc.name, func(t *testing.T) {
			input := prefix + tc.input
			want := int64(len(prefix)) + expectedOffset(t, tc.input, tc.at)

			readers := map[string]func() io.Reader{
				"reader":  func() io.Reader { return strings.NewReader(input) },
				"onebyte": func() io.Reader { return iotest.OneByteReader(strings.NewReader(input)) },
			}
			for name, mk := range readers {
				t.Run(name, func(t *testing.T) {
					dec := NewDecoder(mk())
					for i := 0; i < nFiller; i++ {
						var v map[string]any
						if err := dec.Decode(&v); err != nil {
							t.Fatalf("filler %d: %v", i, err)
						}
					}
					checkOffset(t, dec.Decode(tc.target()), want)
				})
			}
		})
	}
}

// TestSyntaxErrorOffsetLargeDocument fails deep inside a single document
// larger than the read size, so the error token is scanned after refills.
func TestSyntaxErrorOffsetLargeDocument(t *testing.T) {
	var b strings.Builder
	b.WriteString("{\n")
	for i := 0; b.Len() < 3*4096; i++ {
		b.WriteString(`  k` + strings.Repeat("x", 40) + `: [1, 2, 3],` + "\n")
	}
	b.WriteString("  last: 1\n  bad: 2\n}\n")
	input := b.String()
	want := int64(strings.Index(input, "bad: 2"))

	targets := map[string]func() any{
		"map":     func() any { return new(map[string]any) },
		"any":     func() any { return new(any) },
		"ordered": func() any { return new(OrderedMap) },
		"struct":  func() any { return new(offsetStruct) },
	}
	for name, mk := range targets {
		t.Run(name, func(t *testing.T) {
			checkOffset(t, Unmarshal([]byte(input), mk()), want)
			checkOffset(t, NewDecoder(strings.NewReader(input)).Decode(mk()), want)
			checkOffset(t, NewDecoder(iotest.OneByteReader(strings.NewReader(input))).Decode(mk()), want)
		})
	}
}

func TestSyntaxErrorOffsetReproLineColumn(t *testing.T) {
	input := "{\n  body: {\n    fontSize: 11\n    lineHeight: 1.5,\n  },\n}\n"
	err := Unmarshal([]byte(input), new(offsetFontStruct))
	if err == nil || err.Error() != "json5: expected comma in object (offset 33)" {
		t.Fatalf("got %v", err)
	}
	off := int(err.(*SyntaxError).Offset)
	line := strings.Count(input[:off], "\n") + 1
	col := off - strings.LastIndex(input[:off], "\n")
	if line != 4 || col != 5 {
		t.Errorf("line:col = %d:%d, want 4:5", line, col)
	}
}

func TestSyntaxErrorOffsetNestingDepth(t *testing.T) {
	for _, open := range []string{"[", "{a:"} {
		input := strings.Repeat(open, maxNestingDepth+1)
		// The bracket that exceeds the limit is the (maxNestingDepth+1)th.
		want := int64(maxNestingDepth * len(open))
		checkOffset(t, Unmarshal([]byte(input), new(any)), want)
		checkOffset(t, NewDecoder(strings.NewReader(input)).Decode(new(any)), want)
	}
}

func TestSyntaxErrorOffsetTrailing(t *testing.T) {
	input := "{a: 1} x"
	checkOffset(t, Unmarshal([]byte(input), new(any)), int64(strings.Index(input, "x")))
}
