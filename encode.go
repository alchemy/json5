package json5

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"unicode/utf8"
)

var encodeStatePool = sync.Pool{
	New: func() any { return new(encodeState) },
}

func newEncodeState() *encodeState {
	e := encodeStatePool.Get().(*encodeState)
	e.Reset()
	return e
}

type encodeState struct {
	bytes.Buffer
}

func (e *encodeState) marshal(v any) error {
	return e.reflectValue(reflect.ValueOf(v))
}

func (e *encodeState) reflectValue(rv reflect.Value) error {
	if !rv.IsValid() {
		e.WriteString("null")
		return nil
	}
	return e.encodeValue(rv)
}

func (e *encodeState) encodeValue(rv reflect.Value) error {
	// Dereference pointers and interfaces.
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			e.WriteString("null")
			return nil
		}
		rv = rv.Elem()
	}

	// Check for marshaler interfaces.
	if rv.CanAddr() {
		ptr := rv.Addr()
		if ptr.Type().Implements(marshalerType) {
			return e.callMarshaler(ptr, "MarshalJSON5")
		}
		if ptr.Type().Implements(jsonMarshalerType) {
			return e.callJSONMarshaler(ptr)
		}
		if ptr.Type().Implements(textMarshalerType) {
			return e.callTextMarshaler(ptr)
		}
	}
	if rv.Type().Implements(marshalerType) {
		return e.callMarshaler(rv, "MarshalJSON5")
	}
	if rv.Type().Implements(jsonMarshalerType) {
		return e.callJSONMarshaler(rv)
	}
	if rv.Type().Implements(textMarshalerType) {
		return e.callTextMarshaler(rv)
	}

	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			e.WriteString("true")
		} else {
			e.WriteString("false")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.WriteString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32:
		return e.encodeFloat(rv.Float(), 32)
	case reflect.Float64:
		return e.encodeFloat(rv.Float(), 64)
	case reflect.String:
		if rv.Type() == numberType {
			numStr := rv.String()
			if numStr == "" {
				numStr = "0"
			}
			e.WriteString(numStr)
			return nil
		}
		e.encodeString(rv.String())
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			// []byte -> base64.
			return e.encodeBytes(rv)
		}
		if rv.IsNil() {
			e.WriteString("null")
			return nil
		}
		return e.encodeArray(rv)
	case reflect.Array:
		return e.encodeArray(rv)
	case reflect.Map:
		if rv.IsNil() {
			e.WriteString("null")
			return nil
		}
		return e.encodeMap(rv)
	case reflect.Struct:
		return e.encodeStruct(rv)
	default:
		return fmt.Errorf("json5: unsupported type: %s", rv.Type())
	}
	return nil
}

func (e *encodeState) callMarshaler(rv reflect.Value, name string) error {
	m := rv.MethodByName(name)
	ret := m.Call(nil)
	if !ret[1].IsNil() {
		return &MarshalerError{
			Type:       rv.Type(),
			Err:        ret[1].Interface().(error),
			sourceFunc: name,
		}
	}
	b := ret[0].Bytes()
	e.Write(b)
	return nil
}

func (e *encodeState) callJSONMarshaler(rv reflect.Value) error {
	b, err := rv.Interface().(json.Marshaler).MarshalJSON()
	if err != nil {
		return &MarshalerError{Type: rv.Type(), Err: err, sourceFunc: "MarshalJSON"}
	}
	e.Write(b)
	return nil
}

func (e *encodeState) callTextMarshaler(rv reflect.Value) error {
	b, err := rv.Interface().(encoding.TextMarshaler).MarshalText()
	if err != nil {
		return &MarshalerError{Type: rv.Type(), Err: err, sourceFunc: "MarshalText"}
	}
	e.encodeString(string(b))
	return nil
}

func (e *encodeState) encodeFloat(f float64, bits int) error {
	if math.IsInf(f, 1) {
		e.WriteString("Infinity")
		return nil
	}
	if math.IsInf(f, -1) {
		e.WriteString("-Infinity")
		return nil
	}
	if math.IsNaN(f) {
		e.WriteString("NaN")
		return nil
	}
	b := strconv.AppendFloat(nil, f, 'f', -1, bits)
	e.Write(b)
	return nil
}

func (e *encodeState) encodeString(s string) {
	e.WriteByte('"')
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			switch b {
			case '"':
				e.WriteString(`\"`)
			case '\\':
				e.WriteString(`\\`)
			case '\b':
				e.WriteString(`\b`)
			case '\f':
				e.WriteString(`\f`)
			case '\n':
				e.WriteString(`\n`)
			case '\r':
				e.WriteString(`\r`)
			case '\t':
				e.WriteString(`\t`)
			default:
				if b < 0x20 {
					e.WriteString(`\u00`)
					e.WriteByte("0123456789abcdef"[b>>4])
					e.WriteByte("0123456789abcdef"[b&0xF])
				} else {
					e.WriteByte(b)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			e.WriteString(`\ufffd`)
		} else if r == '\u2028' {
			e.WriteString(`\u2028`)
		} else if r == '\u2029' {
			e.WriteString(`\u2029`)
		} else {
			e.WriteString(s[i : i+size])
		}
		i += size
	}
	e.WriteByte('"')
}

func (e *encodeState) encodeBytes(rv reflect.Value) error {
	if rv.IsNil() {
		e.WriteString("null")
		return nil
	}
	// Use encoding/json to produce base64.
	b, err := json.Marshal(rv.Bytes())
	if err != nil {
		return err
	}
	e.Write(b)
	return nil
}

func (e *encodeState) encodeArray(rv reflect.Value) error {
	e.WriteByte('[')
	n := rv.Len()
	for i := 0; i < n; i++ {
		if i > 0 {
			e.WriteByte(',')
		}
		if err := e.encodeValue(rv.Index(i)); err != nil {
			return err
		}
	}
	e.WriteByte(']')
	return nil
}

func (e *encodeState) encodeMap(rv reflect.Value) error {
	e.WriteByte('{')
	keys := rv.MapKeys()
	sortMapKeys(keys)
	for i, k := range keys {
		if i > 0 {
			e.WriteByte(',')
		}
		keyStr, err := resolveMapKeyStr(k)
		if err != nil {
			return err
		}
		e.encodeString(keyStr)
		e.WriteByte(':')
		if err := e.encodeValue(rv.MapIndex(k)); err != nil {
			return err
		}
	}
	e.WriteByte('}')
	return nil
}

func resolveMapKeyStr(k reflect.Value) (string, error) {
	if k.Kind() == reflect.String {
		return k.String(), nil
	}
	if k.Type().Implements(textMarshalerType) {
		b, err := k.Interface().(encoding.TextMarshaler).MarshalText()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	if k.CanAddr() && k.Addr().Type().Implements(textMarshalerType) {
		b, err := k.Addr().Interface().(encoding.TextMarshaler).MarshalText()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	switch k.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	}
	return fmt.Sprintf("%v", k.Interface()), nil
}

func sortMapKeys(keys []reflect.Value) {
	if len(keys) == 0 {
		return
	}
	sort.Slice(keys, func(i, j int) bool {
		ki, _ := resolveMapKeyStr(keys[i])
		kj, _ := resolveMapKeyStr(keys[j])
		return ki < kj
	})
}

func (e *encodeState) encodeStruct(rv reflect.Value) error {
	e.WriteByte('{')
	fields := cachedTypeFields(rv.Type())
	first := true
	for _, f := range fields {
		fv := fieldByIndex(rv, f.index)
		if !fv.IsValid() {
			continue
		}
		if f.omitEmpty && isEmptyValue(fv) {
			continue
		}
		if !first {
			e.WriteByte(',')
		}
		first = false
		e.encodeString(f.name)
		e.WriteByte(':')

		if f.isString {
			if err := e.encodeStringWrapped(fv); err != nil {
				return err
			}
		} else {
			if err := e.encodeValue(fv); err != nil {
				return err
			}
		}
	}
	e.WriteByte('}')
	return nil
}

func (e *encodeState) encodeStringWrapped(rv reflect.Value) error {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			e.WriteString("null")
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Bool:
		if rv.Bool() {
			e.encodeString("true")
		} else {
			e.encodeString("false")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.encodeString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		e.encodeString(strconv.FormatUint(rv.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		e.encodeString(strconv.FormatFloat(rv.Float(), 'f', -1, 64))
	case reflect.String:
		e.encodeString(rv.String())
	default:
		return e.encodeValue(rv)
	}
	return nil
}

func fieldByIndex(rv reflect.Value, index []int) reflect.Value {
	for _, i := range index {
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				return reflect.Value{}
			}
			rv = rv.Elem()
		}
		rv = rv.Field(i)
	}
	return rv
}

func isEmptyValue(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return rv.IsNil()
	}
	return false
}

// indentJSON5 adds indentation to JSON5 output. Unlike json.Indent, this
// handles JSON5-specific tokens like Infinity and NaN.
func indentJSON5(dst *bytes.Buffer, src []byte, prefix, indent string) error {
	sc := &scanner{data: src, preserveComments: true}
	var depth int
	var needNewline bool
	var wroteContent bool // true once any key/value has been written at some depth

	for {
		tok, err := sc.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenEOF {
			break
		}

		switch tok.typ {
		case tokenComment:
			if tok.value == "inline" {
				// Inline comment: append on the same line as the previous value.
				dst.WriteByte(' ')
				dst.WriteString(tok.raw)
			} else {
				// Head comment: starts on its own line, indented to current depth.
				dst.WriteByte('\n')
				writeIndent(dst, prefix, indent, depth)
				dst.WriteString(tok.raw)
				needNewline = true
			}
		case tokenObjectOpen, tokenArrayOpen:
			if needNewline {
				dst.WriteByte('\n')
				writeIndent(dst, prefix, indent, depth)
				needNewline = false
			}
			dst.WriteString(tok.raw)
			depth++
			needNewline = true
			wroteContent = false
		case tokenObjectClose, tokenArrayClose:
			depth--
			if wroteContent || !needNewline {
				dst.WriteByte('\n')
				writeIndent(dst, prefix, indent, depth)
			}
			needNewline = false
			dst.WriteString(tok.raw)
		case tokenComma:
			dst.WriteByte(',')
			needNewline = true
		case tokenColon:
			dst.WriteString(": ")
		default:
			if needNewline {
				dst.WriteByte('\n')
				writeIndent(dst, prefix, indent, depth)
				needNewline = false
			}
			wroteContent = true
			dst.WriteString(tok.raw)
		}
	}
	return nil
}

func writeIndent(dst *bytes.Buffer, prefix, indent string, depth int) {
	dst.WriteString(prefix)
	for i := 0; i < depth; i++ {
		dst.WriteString(indent)
	}
}
