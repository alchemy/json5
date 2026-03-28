package json5

import (
	"fmt"
	"io"
	"unicode"
	"unicode/utf8"
)

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenObjectOpen
	tokenObjectClose
	tokenArrayOpen
	tokenArrayClose
	tokenColon
	tokenComma
	tokenString
	tokenNumber
	tokenTrue
	tokenFalse
	tokenNull
	tokenIdentifier
)

type token struct {
	typ   tokenType
	value string // decoded string value for strings, decoded ident for identifiers, raw text otherwise
	raw   string // raw source text
	pos   int    // absolute byte offset in source
}

type scanner struct {
	data []byte
	pos  int
	r    io.Reader // optional reader for streaming (nil for Unmarshal)
	eof  bool      // reader exhausted
	base int64     // bytes discarded so far, for absolute offsets
}

// absPos returns the absolute byte offset in the input stream.
func (s *scanner) absPos() int {
	return int(s.base) + s.pos
}

func (s *scanner) error(msg string) *SyntaxError {
	return &SyntaxError{msg: msg, Offset: int64(s.absPos())}
}

// fill reads more data from the reader into the buffer. It only appends
// to data; it never shifts existing bytes. Returns true if new data was added.
func (s *scanner) fill() bool {
	if s.r == nil || s.eof {
		return false
	}
	var buf [4096]byte
	n, err := s.r.Read(buf[:])
	if n > 0 {
		s.data = append(s.data, buf[:n]...)
	}
	if err != nil {
		s.eof = true
	}
	return n > 0
}

// compact discards already-consumed bytes from the front of the buffer.
// Must only be called between tokens, when no local variables hold
// positions into data (e.g. at the start of scan).
func (s *scanner) compact() {
	if s.r == nil || s.pos == 0 {
		return
	}
	s.base += int64(s.pos)
	n := copy(s.data, s.data[s.pos:])
	s.data = s.data[:n]
	s.pos = 0
}

// available reports whether there are bytes to process, reading more
// from r if needed.
func (s *scanner) available() bool {
	return s.pos < len(s.data) || s.fill()
}

// ensure ensures at least n bytes are available from the current position,
// reading more from r if needed.
func (s *scanner) ensure(n int) bool {
	for len(s.data)-s.pos < n {
		if !s.fill() {
			return false
		}
	}
	return true
}

// isJSON5Whitespace reports whether r is a JSON5 whitespace character.
func isJSON5Whitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00A0', '\u2028', '\u2029', '\uFEFF':
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

// skipWhitespace skips whitespace and comments.
func (s *scanner) skipWhitespace() error {
	for s.available() {
		b := s.data[s.pos]
		// Fast path for ASCII whitespace.
		switch b {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			s.pos++
			continue
		}
		if b == '/' {
			if !s.ensure(2) {
				return nil
			}
			next := s.data[s.pos+1]
			if next == '/' {
				s.pos += 2
				if err := s.skipLineComment(); err != nil {
					return err
				}
				continue
			}
			if next == '*' {
				s.pos += 2
				if err := s.skipBlockComment(); err != nil {
					return err
				}
				continue
			}
			return nil
		}
		// Multi-byte whitespace (U+00A0, U+2028, U+2029, U+FEFF, Zs).
		if b >= 0x80 {
			s.ensure(utf8.UTFMax)
			r, size := utf8.DecodeRune(s.data[s.pos:])
			if isJSON5Whitespace(r) {
				s.pos += size
				continue
			}
			return nil
		}
		return nil
	}
	return nil
}

func (s *scanner) skipLineComment() error {
	for s.available() {
		b := s.data[s.pos]
		s.pos++
		if b == '\n' || b == '\r' {
			return nil
		}
		// U+2028 (E2 80 A8) and U+2029 (E2 80 A9) are line terminators.
		if b == 0xE2 && s.ensure(2) &&
			s.data[s.pos] == 0x80 && (s.data[s.pos+1] == 0xA8 || s.data[s.pos+1] == 0xA9) {
			s.pos += 2
			return nil
		}
	}
	return nil
}

func (s *scanner) skipBlockComment() error {
	for s.available() {
		if s.data[s.pos] == '*' && s.ensure(2) && s.data[s.pos+1] == '/' {
			s.pos += 2
			return nil
		}
		s.pos++
	}
	return s.error("unterminated block comment")
}

// scan returns the next token.
func (s *scanner) scan() (token, error) {
	if err := s.skipWhitespace(); err != nil {
		return token{}, err
	}
	s.compact()

	if !s.available() {
		return token{typ: tokenEOF, pos: s.absPos()}, nil
	}

	apos := s.absPos()
	ch := s.data[s.pos]

	switch ch {
	case '{':
		s.pos++
		return token{typ: tokenObjectOpen, raw: "{", pos: apos}, nil
	case '}':
		s.pos++
		return token{typ: tokenObjectClose, raw: "}", pos: apos}, nil
	case '[':
		s.pos++
		return token{typ: tokenArrayOpen, raw: "[", pos: apos}, nil
	case ']':
		s.pos++
		return token{typ: tokenArrayClose, raw: "]", pos: apos}, nil
	case ':':
		s.pos++
		return token{typ: tokenColon, raw: ":", pos: apos}, nil
	case ',':
		s.pos++
		return token{typ: tokenComma, raw: ",", pos: apos}, nil
	case '"', '\'':
		return s.scanString()
	case '+', '-':
		return s.scanSignedNumber()
	case '.':
		return s.scanNumber()
	default:
		if ch >= '0' && ch <= '9' {
			return s.scanNumber()
		}
		if isIdentStart(rune(ch)) || ch >= 0x80 || ch == '\\' {
			return s.scanIdentifier()
		}
		return token{}, s.error(fmt.Sprintf("unexpected character %q", string(rune(ch))))
	}
}

func (s *scanner) scanString() (token, error) {
	pos := s.pos
	apos := s.absPos()
	quote := s.data[s.pos]
	s.pos++

	var buf []byte
	for s.available() {
		ch := s.data[s.pos]
		if ch == quote {
			s.pos++
			raw := string(s.data[pos:s.pos])
			return token{typ: tokenString, value: string(buf), raw: raw, pos: apos}, nil
		}
		if ch == '\\' {
			s.pos++
			if !s.available() {
				return token{}, s.error("unterminated string escape")
			}
			esc := s.data[s.pos]
			s.pos++
			switch esc {
			case '\'':
				buf = append(buf, '\'')
			case '"':
				buf = append(buf, '"')
			case '\\':
				buf = append(buf, '\\')
			case '/':
				buf = append(buf, '/')
			case 'b':
				buf = append(buf, '\b')
			case 'f':
				buf = append(buf, '\f')
			case 'n':
				buf = append(buf, '\n')
			case 'r':
				buf = append(buf, '\r')
			case 't':
				buf = append(buf, '\t')
			case 'v':
				buf = append(buf, '\v')
			case '0':
				// \0 is null, but \0 followed by a digit is an error.
				if s.available() && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
					return token{}, s.error("\\0 followed by a digit is not allowed")
				}
				buf = append(buf, 0)
			case 'x':
				// \xHH
				if !s.ensure(2) {
					return token{}, s.error("invalid \\x escape")
				}
				h1, ok1 := hexVal(s.data[s.pos])
				h2, ok2 := hexVal(s.data[s.pos+1])
				if !ok1 || !ok2 {
					return token{}, s.error("invalid \\x escape")
				}
				buf = append(buf, byte(h1<<4|h2))
				s.pos += 2
			case 'u':
				// \uHHHH
				if !s.ensure(4) {
					return token{}, s.error("invalid \\u escape")
				}
				var r rune
				for i := 0; i < 4; i++ {
					h, ok := hexVal(s.data[s.pos+i])
					if !ok {
						return token{}, s.error("invalid \\u escape")
					}
					r = r<<4 | rune(h)
				}
				s.pos += 4
				buf = utf8.AppendRune(buf, r)
			case '\n':
				// Line continuation — skip.
			case '\r':
				// Line continuation — also consume following \n if CRLF.
				if s.available() && s.data[s.pos] == '\n' {
					s.pos++
				}
			default:
				// Check for line separators (multi-byte).
				// We already consumed one byte. We need to check if esc starts
				// a multi-byte rune that is a line terminator.
				if esc >= '1' && esc <= '9' {
					return token{}, s.error("invalid escape sequence: octal escapes are not allowed")
				}
				// Back up to the escape character and re-read as a full rune.
				s.pos--
				s.ensure(utf8.UTFMax)
				r, size := utf8.DecodeRune(s.data[s.pos:])
				s.pos += size
				if r == '\u2028' || r == '\u2029' {
					// Line continuation — skip.
					continue
				}
				// Non-escape character: include literally.
				buf = utf8.AppendRune(buf, r)
			}
			continue
		}

		// Handle multi-byte characters.
		if ch >= 0x80 {
			s.ensure(utf8.UTFMax)
			r, size := utf8.DecodeRune(s.data[s.pos:])
			s.pos += size
			buf = utf8.AppendRune(buf, r)
			continue
		}

		// Regular character. Bare newlines inside strings are not allowed.
		if ch == '\n' || ch == '\r' {
			return token{}, s.error("unterminated string (newline in string)")
		}
		buf = append(buf, ch)
		s.pos++
	}
	return token{}, s.error("unterminated string")
}

func (s *scanner) scanSignedNumber() (token, error) {
	pos := s.pos
	apos := s.absPos()
	sign := s.data[s.pos]
	s.pos++ // skip + or -

	if !s.available() {
		return token{}, s.error("unexpected end of input after sign")
	}

	ch := s.data[s.pos]

	// Check for Infinity or NaN after sign.
	if isIdentStart(rune(ch)) || ch == '\\' {
		raw := s.readIdent()
		value := decodeIdent(raw)
		if value == "Infinity" || value == "NaN" {
			fullRaw := string(s.data[pos:s.pos])
			fullValue := string(sign) + value
			return token{typ: tokenNumber, value: fullValue, raw: fullRaw, pos: apos}, nil
		}
		return token{}, s.error(fmt.Sprintf("unexpected identifier %q after sign", value))
	}

	if ch == '.' || (ch >= '0' && ch <= '9') {
		s.pos = pos // Reset and let scanNumber handle the sign.
		return s.scanNumber()
	}

	return token{}, s.error(fmt.Sprintf("unexpected character %q after sign", string(rune(ch))))
}

func (s *scanner) scanNumber() (token, error) {
	pos := s.pos
	apos := s.absPos()

	// Optional sign.
	if s.available() && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
		s.pos++
	}

	if !s.available() {
		return token{}, s.error("unexpected end of input in number")
	}

	ch := s.data[s.pos]

	// Hexadecimal.
	if ch == '0' && s.ensure(2) && (s.data[s.pos+1] == 'x' || s.data[s.pos+1] == 'X') {
		s.pos += 2
		if !s.available() || !isHexDigit(s.data[s.pos]) {
			return token{}, s.error("invalid hex literal")
		}
		for s.available() && isHexDigit(s.data[s.pos]) {
			s.pos++
		}
		raw := string(s.data[pos:s.pos])
		return token{typ: tokenNumber, value: raw, raw: raw, pos: apos}, nil
	}

	// Integer part (may be absent if starts with '.').
	if ch >= '0' && ch <= '9' {
		start := s.pos
		for s.available() && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
		// JSON5 disallows octal literals: a leading '0' followed by more
		// digits is illegal (e.g. 010, 080). A leading '0' must be followed
		// by '.', 'e'/'E', 'x'/'X', or end of the number.
		if s.data[start] == '0' && s.pos-start > 1 {
			return token{}, s.error("octal literals are not allowed in JSON5")
		}
	}

	// Fractional part.
	if s.available() && s.data[s.pos] == '.' {
		s.pos++
		for s.available() && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}

	// Exponent.
	if s.available() && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.available() && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if !s.available() || s.data[s.pos] < '0' || s.data[s.pos] > '9' {
			return token{}, s.error("invalid exponent in number")
		}
		for s.available() && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}

	if s.pos == pos || (s.pos == pos+1 && (s.data[pos] == '+' || s.data[pos] == '-')) {
		return token{}, s.error("invalid number")
	}

	raw := string(s.data[pos:s.pos])
	return token{typ: tokenNumber, value: raw, raw: raw, pos: apos}, nil
}

func (s *scanner) scanIdentifier() (token, error) {
	apos := s.absPos()
	raw := s.readIdent()
	value := decodeIdent(raw)

	switch value {
	case "true":
		return token{typ: tokenTrue, value: value, raw: raw, pos: apos}, nil
	case "false":
		return token{typ: tokenFalse, value: value, raw: raw, pos: apos}, nil
	case "null":
		return token{typ: tokenNull, value: value, raw: raw, pos: apos}, nil
	case "Infinity":
		return token{typ: tokenNumber, value: value, raw: raw, pos: apos}, nil
	case "NaN":
		return token{typ: tokenNumber, value: value, raw: raw, pos: apos}, nil
	default:
		return token{typ: tokenIdentifier, value: value, raw: raw, pos: apos}, nil
	}
}

// readIdent reads an ECMAScript IdentifierName from the source (raw bytes,
// including any \uHHHH escape sequences verbatim).
func (s *scanner) readIdent() string {
	start := s.pos
	first := true
	for s.available() {
		if s.data[s.pos] == '\\' {
			// Unicode escape in identifier: \uHHHH
			if s.ensure(6) && s.data[s.pos+1] == 'u' {
				valid := true
				var r rune
				for i := 0; i < 4; i++ {
					h, ok := hexVal(s.data[s.pos+2+i])
					if !ok {
						valid = false
						break
					}
					r = r<<4 | rune(h)
				}
				if !valid {
					break
				}
				// Validate the decoded rune is a valid identifier character.
				if first && !isIdentStart(r) {
					break
				}
				if !first && !isIdentPart(r) {
					break
				}
				s.pos += 6
				first = false
				continue
			}
			break
		}
		s.ensure(utf8.UTFMax)
		r, size := utf8.DecodeRune(s.data[s.pos:])
		if first {
			if !isIdentStart(r) {
				break
			}
		} else {
			if !isIdentPart(r) {
				break
			}
		}
		s.pos += size
		first = false
	}
	return string(s.data[start:s.pos])
}

// decodeIdent resolves \uHHHH escape sequences in an identifier string.
func decodeIdent(raw string) string {
	hasEscape := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' {
			hasEscape = true
			break
		}
	}
	if !hasEscape {
		return raw
	}
	var buf []byte
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+5 < len(raw) && raw[i+1] == 'u' {
			var r rune
			for j := 0; j < 4; j++ {
				h, _ := hexVal(raw[i+2+j])
				r = r<<4 | rune(h)
			}
			buf = utf8.AppendRune(buf, r)
			i += 6
		} else {
			r, size := utf8.DecodeRuneInString(raw[i:])
			buf = utf8.AppendRune(buf, r)
			i += size
		}
	}
	return string(buf)
}

// isIdentStart checks whether r is a valid ECMAScript IdentifierStart.
func isIdentStart(r rune) bool {
	if r == '_' || r == '$' {
		return true
	}
	return unicode.IsLetter(r)
}

// isIdentPart checks whether r is a valid ECMAScript IdentifierPart.
func isIdentPart(r rune) bool {
	if isIdentStart(r) {
		return true
	}
	if unicode.IsDigit(r) {
		return true
	}
	// Zero-width joiner and non-joiner.
	if r == '\u200C' || r == '\u200D' {
		return true
	}
	return false
}

func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func hexVal(b byte) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0'), true
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10, true
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10, true
	}
	return 0, false
}
