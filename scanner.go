package json5

import (
	"fmt"
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
	pos   int    // byte offset in source
}

type scanner struct {
	data []byte
	pos  int
}

func (s *scanner) error(msg string) *SyntaxError {
	return &SyntaxError{msg: msg, Offset: int64(s.pos)}
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
	for s.pos < len(s.data) {
		r, size := utf8.DecodeRune(s.data[s.pos:])
		if r == '/' {
			// Check for comment.
			if s.pos+1 < len(s.data) {
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
			}
			return nil
		}
		if isJSON5Whitespace(r) {
			s.pos += size
			continue
		}
		return nil
	}
	return nil
}

func (s *scanner) skipLineComment() error {
	for s.pos < len(s.data) {
		r, size := utf8.DecodeRune(s.data[s.pos:])
		s.pos += size
		if r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029' {
			return nil
		}
	}
	return nil
}

func (s *scanner) skipBlockComment() error {
	for s.pos < len(s.data) {
		if s.data[s.pos] == '*' && s.pos+1 < len(s.data) && s.data[s.pos+1] == '/' {
			s.pos += 2
			return nil
		}
		_, size := utf8.DecodeRune(s.data[s.pos:])
		s.pos += size
	}
	return s.error("unterminated block comment")
}

// scan returns the next token.
func (s *scanner) scan() (token, error) {
	if err := s.skipWhitespace(); err != nil {
		return token{}, err
	}

	if s.pos >= len(s.data) {
		return token{typ: tokenEOF, pos: s.pos}, nil
	}

	pos := s.pos
	ch := s.data[s.pos]

	switch ch {
	case '{':
		s.pos++
		return token{typ: tokenObjectOpen, raw: "{", pos: pos}, nil
	case '}':
		s.pos++
		return token{typ: tokenObjectClose, raw: "}", pos: pos}, nil
	case '[':
		s.pos++
		return token{typ: tokenArrayOpen, raw: "[", pos: pos}, nil
	case ']':
		s.pos++
		return token{typ: tokenArrayClose, raw: "]", pos: pos}, nil
	case ':':
		s.pos++
		return token{typ: tokenColon, raw: ":", pos: pos}, nil
	case ',':
		s.pos++
		return token{typ: tokenComma, raw: ",", pos: pos}, nil
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
	quote := s.data[s.pos]
	s.pos++

	var buf []byte
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		if ch == quote {
			s.pos++
			raw := string(s.data[pos:s.pos])
			return token{typ: tokenString, value: string(buf), raw: raw, pos: pos}, nil
		}
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.data) {
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
				if s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
					return token{}, s.error("\\0 followed by a digit is not allowed")
				}
				buf = append(buf, 0)
			case 'x':
				// \xHH
				if s.pos+2 > len(s.data) {
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
				if s.pos+4 > len(s.data) {
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
				if s.pos < len(s.data) && s.data[s.pos] == '\n' {
					s.pos++
				}
			default:
				// Check for line separators (multi-byte).
				// We already consumed one byte. We need to check if esc starts
				// a multi-byte rune that is a line terminator.
				if esc >= '1' && esc <= '9' {
					return token{}, s.error("invalid escape sequence: octal escapes are not allowed")
				}
				// For U+2028 and U+2029 (line continuation), we need to check
				// if the previous position was the start of such a rune.
				// Since we already consumed esc as a single byte, we need to
				// back up and re-read as a rune.
				s.pos-- // back up to esc
				s.pos-- // back up to backslash
				s.pos++ // skip backslash
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
			r, size := utf8.DecodeRune(s.data[s.pos:])
			s.pos += size
			buf = utf8.AppendRune(buf, r)
			continue
		}

		// Regular character. Line terminators inside strings are allowed in
		// JSON5 only via line continuation; bare newlines inside strings are
		// technically allowed per the grammar for U+2028/U+2029 but not for
		// U+000A/U+000D.
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
	sign := s.data[s.pos]
	s.pos++ // skip + or -

	if s.pos >= len(s.data) {
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
			return token{typ: tokenNumber, value: fullValue, raw: fullRaw, pos: pos}, nil
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

	// Optional sign.
	if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
		s.pos++
	}

	if s.pos >= len(s.data) {
		return token{}, s.error("unexpected end of input in number")
	}

	ch := s.data[s.pos]

	// Hexadecimal.
	if ch == '0' && s.pos+1 < len(s.data) && (s.data[s.pos+1] == 'x' || s.data[s.pos+1] == 'X') {
		s.pos += 2
		if s.pos >= len(s.data) || !isHexDigit(s.data[s.pos]) {
			return token{}, s.error("invalid hex literal")
		}
		for s.pos < len(s.data) && isHexDigit(s.data[s.pos]) {
			s.pos++
		}
		raw := string(s.data[pos:s.pos])
		return token{typ: tokenNumber, value: raw, raw: raw, pos: pos}, nil
	}

	// Integer part (may be absent if starts with '.').
	if ch >= '0' && ch <= '9' {
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}

	// Fractional part.
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}

	// Exponent.
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if s.pos >= len(s.data) || s.data[s.pos] < '0' || s.data[s.pos] > '9' {
			return token{}, s.error("invalid exponent in number")
		}
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
	}

	if s.pos == pos || (s.pos == pos+1 && (s.data[pos] == '+' || s.data[pos] == '-')) {
		return token{}, s.error("invalid number")
	}

	raw := string(s.data[pos:s.pos])
	return token{typ: tokenNumber, value: raw, raw: raw, pos: pos}, nil
}

func (s *scanner) scanIdentifier() (token, error) {
	pos := s.pos
	raw := s.readIdent()
	value := decodeIdent(raw)

	switch value {
	case "true":
		return token{typ: tokenTrue, value: value, raw: raw, pos: pos}, nil
	case "false":
		return token{typ: tokenFalse, value: value, raw: raw, pos: pos}, nil
	case "null":
		return token{typ: tokenNull, value: value, raw: raw, pos: pos}, nil
	case "Infinity":
		return token{typ: tokenNumber, value: value, raw: raw, pos: pos}, nil
	case "NaN":
		return token{typ: tokenNumber, value: value, raw: raw, pos: pos}, nil
	default:
		return token{typ: tokenIdentifier, value: value, raw: raw, pos: pos}, nil
	}
}

// readIdent reads an ECMAScript IdentifierName from the source (raw bytes,
// including any \uHHHH escape sequences verbatim).
func (s *scanner) readIdent() string {
	start := s.pos
	first := true
	for s.pos < len(s.data) {
		if s.data[s.pos] == '\\' {
			// Unicode escape in identifier: \uHHHH
			if s.pos+6 <= len(s.data) && s.data[s.pos+1] == 'u' {
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
