package json5

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const maxNestingDepth = 10000

type decodeState struct {
	scanner               scanner
	useNumber             bool
	disallowUnknownFields bool
	depth                 int
}

func (d *decodeState) init(data []byte) {
	d.scanner = scanner{data: data}
}

func (d *decodeState) unmarshal(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return &InvalidUnmarshalError{Type: reflect.TypeOf(v)}
	}
	tok, err := d.scanner.scan()
	if err != nil {
		return err
	}
	return d.value(tok, rv.Elem())
}

func (d *decodeState) value(tok token, rv reflect.Value) error {
	// Check for unmarshaler interfaces on the pointer to rv.
	if rv.IsValid() && rv.CanAddr() {
		ptr := rv.Addr()
		if ptr.Type().Implements(unmarshalerType) {
			raw, err := d.collectRaw(tok)
			if err != nil {
				return err
			}
			return ptr.Interface().(Unmarshaler).UnmarshalJSON5(raw)
		}
		if ptr.Type().Implements(jsonUnmarshalerType) {
			raw, err := d.collectRaw(tok)
			if err != nil {
				return err
			}
			return ptr.Interface().(json.Unmarshaler).UnmarshalJSON(raw)
		}
	}

	switch tok.typ {
	case tokenObjectOpen:
		return d.object(rv)
	case tokenArrayOpen:
		return d.array(rv)
	case tokenString:
		return d.literalString(tok, rv)
	case tokenNumber:
		return d.literalNumber(tok, rv)
	case tokenTrue:
		return d.literalBool(tok, rv, true)
	case tokenFalse:
		return d.literalBool(tok, rv, false)
	case tokenNull:
		return d.literalNull(rv)
	case tokenEOF:
		return d.scanner.error("unexpected end of input")
	default:
		return d.scanner.error(fmt.Sprintf("unexpected token %q", tok.raw))
	}
}

// collectRaw re-scans from the token's position to capture the raw JSON5 for
// a complete value. For objects and arrays it balances braces/brackets.
// For scalar tokens it returns the raw token text, converting to JSON-compatible
// format for json.Unmarshaler compatibility.
func (d *decodeState) collectRaw(tok token) ([]byte, error) {
	switch tok.typ {
	case tokenString:
		// Re-encode as a JSON string for compatibility.
		b, _ := json.Marshal(tok.value)
		return b, nil
	case tokenNumber:
		return normalizeNumber(tok.value), nil
	case tokenTrue:
		return []byte("true"), nil
	case tokenFalse:
		return []byte("false"), nil
	case tokenNull:
		return []byte("null"), nil
	case tokenObjectOpen, tokenArrayOpen:
		return d.collectRawComposite(tok)
	default:
		return []byte(tok.raw), nil
	}
}

func (d *decodeState) collectRawComposite(tok token) ([]byte, error) {
	d.depth++
	if d.depth > maxNestingDepth {
		return nil, d.scanner.error("exceeded max nesting depth")
	}
	defer func() { d.depth-- }()

	var buf []byte
	if tok.typ == tokenObjectOpen {
		buf = append(buf, '{')
		first := true
		for {
			t, err := d.scanner.scan()
			if err != nil {
				return nil, err
			}
			if t.typ == tokenObjectClose {
				buf = append(buf, '}')
				return buf, nil
			}
			if !first {
				if t.typ != tokenComma {
					return nil, d.scanner.error("expected comma in object")
				}
				t, err = d.scanner.scan()
				if err != nil {
					return nil, err
				}
				if t.typ == tokenObjectClose {
					// Trailing comma — omit from JSON output.
					buf = append(buf, '}')
					return buf, nil
				}
				buf = append(buf, ',')
			}
			first = false
			// Key
			key, err := d.resolveKey(t)
			if err != nil {
				return nil, err
			}
			jk, _ := json.Marshal(key)
			buf = append(buf, jk...)
			buf = append(buf, ':')
			// Colon
			colon, err := d.scanner.scan()
			if err != nil {
				return nil, err
			}
			if colon.typ != tokenColon {
				return nil, d.scanner.error("expected colon after object key")
			}
			// Value
			vt, err := d.scanner.scan()
			if err != nil {
				return nil, err
			}
			vraw, err := d.collectRaw(vt)
			if err != nil {
				return nil, err
			}
			buf = append(buf, vraw...)
		}
	}

	// Array
	buf = append(buf, '[')
	first := true
	for {
		t, err := d.scanner.scan()
		if err != nil {
			return nil, err
		}
		if t.typ == tokenArrayClose {
			buf = append(buf, ']')
			return buf, nil
		}
		if !first {
			if t.typ != tokenComma {
				return nil, d.scanner.error("expected comma in array")
			}
			t, err = d.scanner.scan()
			if err != nil {
				return nil, err
			}
			if t.typ == tokenArrayClose {
				// Trailing comma — omit from JSON output.
				buf = append(buf, ']')
				return buf, nil
			}
			buf = append(buf, ',')
		}
		first = false
		vraw, err := d.collectRaw(t)
		if err != nil {
			return nil, err
		}
		buf = append(buf, vraw...)
	}
}

// normalizeNumber converts JSON5 number representations to JSON-compatible ones.
func normalizeNumber(s string) []byte {
	switch s {
	case "Infinity", "+Infinity":
		return []byte("1e+999") // parsed as +Inf by Go
	case "-Infinity":
		return []byte("-1e+999")
	case "NaN", "+NaN", "-NaN":
		return []byte("null") // NaN is not representable in JSON; use null
	}
	// Hex -> decimal.
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		n, err := strconv.ParseInt(s, 0, 64)
		if err == nil {
			return []byte(strconv.FormatInt(n, 10))
		}
		// Try unsigned for large hex values.
		un, err := strconv.ParseUint(s, 0, 64)
		if err == nil {
			return []byte(strconv.FormatUint(un, 10))
		}
	}
	// Signed hex.
	if len(s) > 3 && (s[0] == '+' || s[0] == '-') && s[1] == '0' && (s[2] == 'x' || s[2] == 'X') {
		n, err := strconv.ParseInt(s[1:], 0, 64)
		if err == nil {
			if s[0] == '-' {
				n = -n
			}
			return []byte(strconv.FormatInt(n, 10))
		}
	}
	// Leading '+' sign.
	if len(s) > 0 && s[0] == '+' {
		return []byte(s[1:])
	}
	// Leading dot -> "0."
	if len(s) > 0 && s[0] == '.' {
		return []byte("0" + s)
	}
	// Negative with leading dot.
	if len(s) > 1 && s[0] == '-' && s[1] == '.' {
		return []byte("-0" + s[1:])
	}
	// Trailing dot -> remove dot.
	if strings.HasSuffix(s, ".") && !strings.ContainsAny(s, "eE") {
		return []byte(s[:len(s)-1])
	}
	return []byte(s)
}

func (d *decodeState) object(rv reflect.Value) error {
	d.depth++
	if d.depth > maxNestingDepth {
		return d.scanner.error("exceeded max nesting depth")
	}
	defer func() { d.depth-- }()

	// Handle interface{}.
	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		m := make(map[string]any)
		mv := reflect.ValueOf(&m).Elem()
		if err := d.object(mv); err != nil {
			return err
		}
		rv.Set(mv)
		return nil
	}

	// Handle pointer.
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.object(rv.Elem())
	}

	switch rv.Kind() {
	case reflect.Map:
		return d.objectMap(rv)
	case reflect.Struct:
		return d.objectStruct(rv)
	default:
		// Skip the object value.
		return d.skipObject()
	}
}

func (d *decodeState) objectMap(rv reflect.Value) error {
	t := rv.Type()
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(t))
	}
	keyType := t.Key()
	elemType := t.Elem()

	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenObjectClose {
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in object")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenObjectClose {
				return nil // trailing comma
			}
		}
		first = false

		// Parse key.
		keyStr, err := d.resolveKey(tok)
		if err != nil {
			return err
		}

		// Colon.
		colon, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if colon.typ != tokenColon {
			return d.scanner.error("expected colon after object key")
		}

		// Value.
		valTok, err := d.scanner.scan()
		if err != nil {
			return err
		}

		mapKey, err := d.convertMapKey(keyStr, keyType)
		if err != nil {
			return err
		}

		mapVal := reflect.New(elemType).Elem()
		if err := d.value(valTok, mapVal); err != nil {
			return err
		}
		rv.SetMapIndex(mapKey, mapVal)
	}
}

func (d *decodeState) convertMapKey(key string, keyType reflect.Type) (reflect.Value, error) {
	switch keyType.Kind() {
	case reflect.String:
		return reflect.ValueOf(key).Convert(keyType), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("json5: cannot use %q as map key of type %s", key, keyType)
		}
		return reflect.ValueOf(n).Convert(keyType), nil
	default:
		// Check for TextUnmarshaler.
		ptrType := reflect.PointerTo(keyType)
		if ptrType.Implements(textUnmarshalerType) {
			kv := reflect.New(keyType)
			if err := kv.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(key)); err != nil {
				return reflect.Value{}, err
			}
			return kv.Elem(), nil
		}
		return reflect.Value{}, fmt.Errorf("json5: unsupported map key type %s", keyType)
	}
}

func (d *decodeState) objectStruct(rv reflect.Value) error {
	fields := cachedTypeFields(rv.Type())

	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenObjectClose {
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in object")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenObjectClose {
				return nil // trailing comma
			}
		}
		first = false

		keyStr, err := d.resolveKey(tok)
		if err != nil {
			return err
		}

		// Colon.
		colon, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if colon.typ != tokenColon {
			return d.scanner.error("expected colon after object key")
		}

		// Find field.
		var f *field
		for i := range fields {
			if fields[i].name == keyStr || strings.EqualFold(fields[i].name, keyStr) {
				f = &fields[i]
				break
			}
		}

		valTok, err := d.scanner.scan()
		if err != nil {
			return err
		}

		if f == nil {
			if d.disallowUnknownFields {
				return fmt.Errorf("json5: unknown field %q", keyStr)
			}
			if err := d.skipValue(valTok); err != nil {
				return err
			}
			continue
		}

		subv := rv
		for _, idx := range f.index {
			if subv.Kind() == reflect.Pointer {
				if subv.IsNil() {
					subv.Set(reflect.New(subv.Type().Elem()))
				}
				subv = subv.Elem()
			}
			subv = subv.Field(idx)
		}

		// Handle the ,string tag option: the JSON5 value is a string wrapping
		// a scalar that should be decoded into the target type.
		if f.isString && valTok.typ == tokenString {
			if err := d.decodeStringTag(valTok.value, subv); err != nil {
				return err
			}
			continue
		}

		if err := d.value(valTok, subv); err != nil {
			return err
		}
	}
}

// decodeStringTag handles the ,string struct tag option by re-parsing the
// string content as a JSON5 value into the target reflect.Value.
func (d *decodeState) decodeStringTag(s string, rv reflect.Value) error {
	// For string targets, use the value directly.
	if rv.Kind() == reflect.String {
		rv.SetString(s)
		return nil
	}
	// For other types, re-parse the string content through the decoder.
	inner := &decodeState{}
	inner.init([]byte(s))
	inner.useNumber = d.useNumber
	inner.depth = d.depth
	tok, err := inner.scanner.scan()
	if err != nil {
		return &UnmarshalTypeError{Value: "string", Type: rv.Type()}
	}
	return inner.value(tok, rv)
}

func (d *decodeState) resolveKey(tok token) (string, error) {
	switch tok.typ {
	case tokenString:
		return tok.value, nil
	case tokenIdentifier:
		return tok.value, nil
	// Allow some keyword tokens as keys.
	case tokenTrue:
		return "true", nil
	case tokenFalse:
		return "false", nil
	case tokenNull:
		return "null", nil
	default:
		return "", d.scanner.error(fmt.Sprintf("expected object key, got %q", tok.raw))
	}
}

func (d *decodeState) array(rv reflect.Value) error {
	d.depth++
	if d.depth > maxNestingDepth {
		return d.scanner.error("exceeded max nesting depth")
	}
	defer func() { d.depth-- }()

	// Handle interface{}.
	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		var arr []any
		av := reflect.ValueOf(&arr).Elem()
		if err := d.array(av); err != nil {
			return err
		}
		rv.Set(av)
		return nil
	}

	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.array(rv.Elem())
	}

	switch rv.Kind() {
	case reflect.Slice:
		return d.arraySlice(rv)
	case reflect.Array:
		return d.arrayFixed(rv)
	default:
		return d.skipArray()
	}
}

func (d *decodeState) arraySlice(rv reflect.Value) error {
	elemType := rv.Type().Elem()
	i := 0
	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenArrayClose {
			if rv.IsNil() {
				rv.Set(reflect.MakeSlice(rv.Type(), 0, 0))
			}
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in array")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenArrayClose {
				return nil // trailing comma
			}
		}
		first = false

		// Grow the slice if needed.
		if i >= rv.Len() {
			rv.Set(reflect.Append(rv, reflect.New(elemType).Elem()))
		}
		if err := d.value(tok, rv.Index(i)); err != nil {
			return err
		}
		i++
	}
}

func (d *decodeState) arrayFixed(rv reflect.Value) error {
	i := 0
	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenArrayClose {
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in array")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenArrayClose {
				return nil // trailing comma
			}
		}
		first = false

		if i < rv.Len() {
			if err := d.value(tok, rv.Index(i)); err != nil {
				return err
			}
		} else {
			if err := d.skipValue(tok); err != nil {
				return err
			}
		}
		i++
	}
}

func (d *decodeState) literalString(tok token, rv reflect.Value) error {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.literalString(tok, rv.Elem())
	}

	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		rv.Set(reflect.ValueOf(tok.value))
		return nil
	}

	// Check for TextUnmarshaler.
	if rv.CanAddr() {
		ptr := rv.Addr()
		if ptr.Type().Implements(textUnmarshalerType) {
			return ptr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(tok.value))
		}
	}

	switch rv.Kind() {
	case reflect.String:
		rv.SetString(tok.value)
	default:
		return &UnmarshalTypeError{Value: "string", Type: rv.Type(), Offset: int64(tok.pos)}
	}
	return nil
}

func (d *decodeState) literalNumber(tok token, rv reflect.Value) error {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.literalNumber(tok, rv.Elem())
	}

	s := tok.value

	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		if d.useNumber {
			rv.Set(reflect.ValueOf(Number(s)))
			return nil
		}
		f, err := parseJSON5Number(s)
		if err != nil {
			return d.scanner.error(fmt.Sprintf("invalid number: %s", s))
		}
		rv.Set(reflect.ValueOf(f))
		return nil
	}

	if rv.Type() == numberType {
		rv.SetString(s)
		return nil
	}

	// Check for TextUnmarshaler.
	if rv.CanAddr() {
		ptr := rv.Addr()
		if ptr.Type().Implements(textUnmarshalerType) {
			return ptr.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(s))
		}
	}

	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := parseJSON5Int(s)
		if err != nil {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		if rv.OverflowInt(n) {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		rv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := parseJSON5Uint(s)
		if err != nil {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		if rv.OverflowUint(n) {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		rv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := parseJSON5Number(s)
		if err != nil {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		if rv.OverflowFloat(f) {
			return &UnmarshalTypeError{Value: "number " + s, Type: rv.Type(), Offset: int64(tok.pos)}
		}
		rv.SetFloat(f)
	case reflect.String:
		rv.SetString(s)
	default:
		return &UnmarshalTypeError{Value: "number", Type: rv.Type(), Offset: int64(tok.pos)}
	}
	return nil
}

func (d *decodeState) literalBool(tok token, rv reflect.Value, b bool) error {
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		return d.literalBool(tok, rv.Elem(), b)
	}

	if rv.Kind() == reflect.Interface && rv.NumMethod() == 0 {
		rv.Set(reflect.ValueOf(b))
		return nil
	}

	switch rv.Kind() {
	case reflect.Bool:
		rv.SetBool(b)
	default:
		return &UnmarshalTypeError{Value: "bool", Type: rv.Type(), Offset: int64(tok.pos)}
	}
	return nil
}

func (d *decodeState) literalNull(rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		rv.Set(reflect.Zero(rv.Type()))
	}
	return nil
}

// skipValue skips over one complete value.
func (d *decodeState) skipValue(tok token) error {
	switch tok.typ {
	case tokenObjectOpen:
		return d.skipObject()
	case tokenArrayOpen:
		return d.skipArray()
	case tokenString, tokenNumber, tokenTrue, tokenFalse, tokenNull:
		return nil
	default:
		return d.scanner.error(fmt.Sprintf("unexpected token %q", tok.raw))
	}
}

func (d *decodeState) skipObject() error {
	d.depth++
	if d.depth > maxNestingDepth {
		return d.scanner.error("exceeded max nesting depth")
	}
	defer func() { d.depth-- }()

	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenObjectClose {
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in object")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenObjectClose {
				return nil
			}
		}
		first = false
		// Skip key.
		// Skip colon.
		colon, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if colon.typ != tokenColon {
			return d.scanner.error("expected colon")
		}
		// Skip value.
		valTok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if err := d.skipValue(valTok); err != nil {
			return err
		}
	}
}

func (d *decodeState) skipArray() error {
	d.depth++
	if d.depth > maxNestingDepth {
		return d.scanner.error("exceeded max nesting depth")
	}
	defer func() { d.depth-- }()

	first := true
	for {
		tok, err := d.scanner.scan()
		if err != nil {
			return err
		}
		if tok.typ == tokenArrayClose {
			return nil
		}
		if !first {
			if tok.typ != tokenComma {
				return d.scanner.error("expected comma in array")
			}
			tok, err = d.scanner.scan()
			if err != nil {
				return err
			}
			if tok.typ == tokenArrayClose {
				return nil
			}
		}
		first = false
		if err := d.skipValue(tok); err != nil {
			return err
		}
	}
}

// parseJSON5Number parses a JSON5 number string to float64.
func parseJSON5Number(s string) (float64, error) {
	// Strip leading '+'.
	clean := s
	if len(clean) > 0 && clean[0] == '+' {
		clean = clean[1:]
	}

	switch clean {
	case "Infinity":
		return math.Inf(1), nil
	case "-Infinity":
		return math.Inf(-1), nil
	case "NaN", "-NaN":
		return math.NaN(), nil
	}

	// Hex.
	if len(clean) > 2 && clean[0] == '0' && (clean[1] == 'x' || clean[1] == 'X') {
		n, err := strconv.ParseInt(clean, 0, 64)
		if err != nil {
			un, err2 := strconv.ParseUint(clean, 0, 64)
			if err2 != nil {
				return 0, err
			}
			return float64(un), nil
		}
		return float64(n), nil
	}
	// Negative hex.
	if len(clean) > 3 && clean[0] == '-' && clean[1] == '0' && (clean[2] == 'x' || clean[2] == 'X') {
		n, err := strconv.ParseInt(clean[1:], 0, 64)
		if err != nil {
			return 0, err
		}
		return float64(-n), nil
	}

	return strconv.ParseFloat(clean, 64)
}

// parseJSON5Int parses a JSON5 number as int64.
func parseJSON5Int(s string) (int64, error) {
	clean := s
	if len(clean) > 0 && clean[0] == '+' {
		clean = clean[1:]
	}
	// Hex.
	if len(clean) > 2 && clean[0] == '0' && (clean[1] == 'x' || clean[1] == 'X') {
		return strconv.ParseInt(clean, 0, 64)
	}
	if len(clean) > 3 && clean[0] == '-' && clean[1] == '0' && (clean[2] == 'x' || clean[2] == 'X') {
		n, err := strconv.ParseInt(clean[1:], 0, 64)
		if err != nil {
			return 0, err
		}
		return -n, nil
	}
	// Handle trailing dot.
	clean = strings.TrimSuffix(clean, ".")
	// Handle leading dot.
	if strings.HasPrefix(clean, ".") || strings.HasPrefix(clean, "-.") {
		// Not a valid integer.
		return 0, fmt.Errorf("not an integer: %s", s)
	}
	return strconv.ParseInt(clean, 10, 64)
}

// parseJSON5Uint parses a JSON5 number as uint64.
func parseJSON5Uint(s string) (uint64, error) {
	clean := s
	if len(clean) > 0 && clean[0] == '+' {
		clean = clean[1:]
	}
	if len(clean) > 2 && clean[0] == '0' && (clean[1] == 'x' || clean[1] == 'X') {
		return strconv.ParseUint(clean, 0, 64)
	}
	clean = strings.TrimSuffix(clean, ".")
	return strconv.ParseUint(clean, 10, 64)
}

// --- Struct field caching ---

type field struct {
	name      string
	index     []int
	omitEmpty bool
	isString  bool
}

var fieldCache sync.Map // map[reflect.Type][]field

func cachedTypeFields(t reflect.Type) []field {
	if f, ok := fieldCache.Load(t); ok {
		return f.([]field)
	}
	f := typeFields(t)
	fieldCache.Store(t, f)
	return f
}

func typeFields(t reflect.Type) []field {
	var fields []field
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() && !sf.Anonymous {
			continue
		}

		tag := sf.Tag.Get("json5")
		if tag == "" {
			tag = sf.Tag.Get("json")
		}
		if tag == "-" {
			continue
		}

		name, opts := parseTag(tag)
		if !isValidTag(name) {
			name = ""
		}
		if name == "" {
			name = sf.Name
		}

		// Handle embedded structs (including pointer-to-struct).
		ft := sf.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if sf.Anonymous && ft.Kind() == reflect.Struct && tag == "" {
			embedded := typeFields(ft)
			for _, ef := range embedded {
				idx := make([]int, len(ef.index)+1)
				idx[0] = i
				copy(idx[1:], ef.index)
				ef.index = idx
				fields = append(fields, ef)
			}
			continue
		}

		fields = append(fields, field{
			name:      name,
			index:     []int{i},
			omitEmpty: opts.Contains("omitempty"),
			isString:  opts.Contains("string"),
		})
	}
	return fields
}
