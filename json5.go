// Package json5 implements encoding and decoding of JSON5 as defined by
// https://spec.json5.org/. The API mirrors the standard library encoding/json
// package so that migrating existing code is straightforward.
//
// JSON5 is a superset of JSON that adds support for comments, trailing commas,
// unquoted object keys, single-quoted strings, hexadecimal numbers, Infinity,
// NaN, and additional whitespace characters.
//
// Struct tags use the key "json5" (falling back to "json" if "json5" is absent)
// and follow the same conventions as encoding/json.
package json5

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Marshaler is the interface implemented by types that can marshal themselves
// into valid JSON5.
type Marshaler interface {
	MarshalJSON5() ([]byte, error)
}

// Unmarshaler is the interface implemented by types that can unmarshal a JSON5
// description of themselves.
type Unmarshaler interface {
	UnmarshalJSON5([]byte) error
}

// RawMessage is a raw encoded JSON5 value. It implements Marshaler and
// Unmarshaler and can be used to delay JSON5 decoding or precompute a JSON5
// encoding.
type RawMessage []byte

// MarshalJSON5 returns m as the JSON5 encoding of m.
func (m RawMessage) MarshalJSON5() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return m, nil
}

// UnmarshalJSON5 sets *m to a copy of data.
func (m *RawMessage) UnmarshalJSON5(data []byte) error {
	if m == nil {
		return fmt.Errorf("json5.RawMessage: UnmarshalJSON5 on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

var _ Marshaler = (*RawMessage)(nil)
var _ Unmarshaler = (*RawMessage)(nil)

// Number represents a JSON5 number literal.
type Number string

// String returns the literal text of the number.
func (n Number) String() string { return string(n) }

// Float64 returns the number as a float64.
// It handles JSON5 number formats including hexadecimal, Infinity, and NaN.
func (n Number) Float64() (float64, error) {
	return parseJSON5Number(string(n))
}

// Int64 returns the number as an int64.
// It handles JSON5 number formats including hexadecimal.
func (n Number) Int64() (int64, error) {
	return parseJSON5Int(string(n))
}

// SyntaxError is a description of a JSON5 syntax error.
type SyntaxError struct {
	msg    string
	Offset int64
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("json5: %s (offset %d)", e.msg, e.Offset)
}

// UnmarshalTypeError describes a JSON5 value that was not appropriate for a
// value of a specific Go type.
type UnmarshalTypeError struct {
	Value  string
	Type   reflect.Type
	Offset int64
	Struct string
	Field  string
}

func (e *UnmarshalTypeError) Error() string {
	if e.Struct != "" || e.Field != "" {
		return fmt.Sprintf("json5: cannot unmarshal %s into Go struct field %s.%s of type %s",
			e.Value, e.Struct, e.Field, e.Type)
	}
	return fmt.Sprintf("json5: cannot unmarshal %s into Go value of type %s", e.Value, e.Type)
}

// InvalidUnmarshalError describes an invalid argument passed to Unmarshal.
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "json5: Unmarshal(nil)"
	}
	if e.Type.Kind() != reflect.Pointer {
		return fmt.Sprintf("json5: Unmarshal(non-pointer %s)", e.Type)
	}
	return fmt.Sprintf("json5: Unmarshal(nil %s)", e.Type)
}

// MarshalerError describes an error from calling MarshalJSON5 or MarshalJSON.
type MarshalerError struct {
	Type       reflect.Type
	Err        error
	sourceFunc string
}

func (e *MarshalerError) Error() string {
	srcFunc := e.sourceFunc
	if srcFunc == "" {
		srcFunc = "MarshalJSON5"
	}
	return fmt.Sprintf("json5: error calling %s for type %s: %s", srcFunc, e.Type, e.Err)
}

func (e *MarshalerError) Unwrap() error { return e.Err }

// Marshal returns the JSON5 encoding of v.
//
// Marshal traverses the value v recursively using the same rules as
// encoding/json.Marshal, but outputs valid JSON (which is also valid JSON5).
func Marshal(v any) ([]byte, error) {
	e := newEncodeState()
	err := e.marshal(v)
	if err != nil {
		return nil, err
	}
	buf := append([]byte(nil), e.Bytes()...)
	encodeStatePool.Put(e)
	return buf, nil
}

// MarshalIndent is like Marshal but applies Indent to format the output.
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	b, err := Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = indentJSON5(&buf, b, prefix, indent)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal parses the JSON5-encoded data and stores the result in the value
// pointed to by v. If v is nil or not a pointer, Unmarshal returns an
// InvalidUnmarshalError.
func Unmarshal(data []byte, v any) error {
	d := &decodeState{}
	d.init(data)
	if err := d.unmarshal(v); err != nil {
		return err
	}
	// Reject trailing non-whitespace/comment content.
	if err := d.scanner.skipWhitespace(); err != nil {
		return err
	}
	if d.scanner.pos < len(d.scanner.data) {
		return &SyntaxError{
			msg:    fmt.Sprintf("unexpected trailing character %q", d.scanner.data[d.scanner.pos]),
			Offset: int64(d.scanner.pos),
		}
	}
	return nil
}

// Valid reports whether data is a valid JSON5 encoding.
func Valid(data []byte) bool {
	d := &decodeState{}
	d.init(data)
	var raw any
	if err := d.unmarshal(&raw); err != nil {
		return false
	}
	// Make sure there's no trailing non-whitespace/comment content.
	d.scanner.skipWhitespace()
	return d.scanner.pos >= len(d.scanner.data)
}

// A Decoder reads and decodes JSON5 values from an input stream.
// It reads input incrementally using a sliding window buffer, so it can
// decode a stream of values without loading the entire input into memory.
type Decoder struct {
	sc                    scanner
	err                   error
	useNumber             bool
	disallowUnknownFields bool
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{sc: scanner{r: r}}
}

// UseNumber causes the Decoder to unmarshal a number into an interface{} as a
// Number instead of as a float64.
func (dec *Decoder) UseNumber() {
	dec.useNumber = true
}

// DisallowUnknownFields causes the Decoder to return an error when the
// destination is a struct and the input contains object keys which do not match
// any non-ignored, exported fields in the destination.
func (dec *Decoder) DisallowUnknownFields() {
	dec.disallowUnknownFields = true
}

// Decode reads the next JSON5 value from its input and stores it in the value
// pointed to by v.
func (dec *Decoder) Decode(v any) error {
	if dec.err != nil {
		return dec.err
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	d := &decodeState{
		scanner:               dec.sc,
		useNumber:             dec.useNumber,
		disallowUnknownFields: dec.disallowUnknownFields,
	}
	tok, err := d.scanner.scan()
	if err != nil {
		dec.sc = d.scanner
		dec.err = err
		return err
	}
	if tok.typ == tokenEOF {
		dec.sc = d.scanner
		dec.err = io.EOF
		return io.EOF
	}
	err = d.value(tok, rv.Elem())
	dec.sc = d.scanner
	if err != nil {
		return err
	}
	return nil
}

// An Encoder writes JSON5 values to an output stream.
type Encoder struct {
	w   io.Writer
	err error
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the JSON5 encoding of v to the stream, followed by a newline.
func (enc *Encoder) Encode(v any) error {
	if enc.err != nil {
		return enc.err
	}
	b, err := Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, enc.err = enc.w.Write(b)
	return enc.err
}

// Interfaces we check in order of priority during encoding/decoding.
var (
	marshalerType     = reflect.TypeOf((*Marshaler)(nil)).Elem()
	unmarshalerType   = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	numberType        = reflect.TypeOf(Number(""))
)
