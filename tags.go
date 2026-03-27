package json5

import (
	"strings"
)

// tagOptions is the string following a comma in a struct field's "json5" or
// "json" tag, or the empty string. It does not include the leading comma.
type tagOptions string

// parseTag splits a struct field's "json5" or "json" tag into its name and
// comma-separated options.
func parseTag(tag string) (string, tagOptions) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tagOptions(tag[idx+1:])
	}
	return tag, ""
}

// Contains reports whether a comma-separated list of options contains a
// particular option.
func (o tagOptions) Contains(optName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var name string
		if i := strings.Index(s, ","); i >= 0 {
			name, s = s[:i], s[i+1:]
		} else {
			name, s = s, ""
		}
		if name == optName {
			return true
		}
	}
	return false
}

// isValidTag checks whether s is a valid tag name. It mirrors the logic from
// encoding/json.
func isValidTag(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c):
			// Acceptable punctuation.
		default:
			if !('a' <= c && c <= 'z') && !('A' <= c && c <= 'Z') && !('0' <= c && c <= '9') {
				return false
			}
		}
	}
	return true
}
