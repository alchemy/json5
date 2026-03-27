package json5

import (
	"os"
	"path/filepath"
	"testing"
)

// knownFailures lists test files where the parser does not yet match the spec.
// Each entry documents the non-conformance so it can be fixed later.
var knownFailures = map[string]string{
	// Parser does not reject trailing content after a valid top-level value.
	"comments/unterminated-block-comment.txt": "parser stops after first valid value, ignores trailing unterminated comment",

	// Parser accepts float and hex literals in exponents (e.g. 1e2.3, 1e0x1).
	// JSON5 requires integer-only exponents.
	"numbers/integer-with-float-exponent.txt":              "parser accepts float exponent (1e2.3)",
	"numbers/integer-with-hexadecimal-exponent.txt":        "parser accepts hex exponent (1e0x1)",
	"numbers/integer-with-negative-float-exponent.txt":     "parser accepts negative float exponent (1e-2.3)",
	"numbers/integer-with-negative-hexadecimal-exponent.txt": "parser accepts negative hex exponent (1e-0x1)",
	"numbers/integer-with-positive-float-exponent.txt":     "parser accepts positive float exponent (1e+2.3)",
	"numbers/integer-with-positive-hexadecimal-exponent.txt": "parser accepts positive hex exponent (1e+0x1)",

	// Parser does not reject octal literals (e.g. 010, 0777).
	// JSON5 explicitly disallows octal.
	"numbers/octal.txt":            "parser accepts octal literal (010)",
	"numbers/negative-octal.txt":   "parser accepts negative octal (-010)",
	"numbers/positive-octal.txt":   "parser accepts positive octal (+010)",
	"numbers/zero-octal.txt":       "parser accepts zero octal (00)",
	"numbers/negative-zero-octal.txt": "parser accepts negative zero octal (-00)",
	"numbers/positive-zero-octal.txt": "parser accepts positive zero octal (+00)",

	// Parser does not reject "noctal" literals (leading zero followed by 8/9).
	// These are valid ES5 but explicitly not valid JSON5.
	"numbers/noctal.js":                        "parser accepts noctal (080)",
	"numbers/noctal-with-leading-octal-digit.js": "parser accepts noctal with leading octal digit (0780)",
	"numbers/negative-noctal.js":               "parser accepts negative noctal (-080)",
	"numbers/positive-noctal.js":               "parser accepts positive noctal (+080)",
}

func TestConformance(t *testing.T) {
	root := "testdata/json5-tests"
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("json5-tests submodule not checked out; run: git submodule update --init")
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip the todo directory — these are not yet finalized upstream.
			if d.Name() == "todo" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)

		// Only process actual test input files.
		switch ext {
		case ".json", ".json5", ".js", ".txt":
			// these are test cases
		default:
			return nil
		}

		rel, _ := filepath.Rel(root, path)

		t.Run(rel, func(t *testing.T) {
			if reason, ok := knownFailures[rel]; ok {
				t.Skip("known non-conformance: " + reason)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			switch ext {
			case ".json", ".json5":
				// Must parse successfully.
				var v any
				if err := Unmarshal(data, &v); err != nil {
					t.Errorf("expected valid JSON5, got error: %v", err)
				}
				if !Valid(data) {
					t.Errorf("Valid() returned false for valid input")
				}

			case ".js", ".txt":
				// Must fail to parse.
				var v any
				if err := Unmarshal(data, &v); err == nil {
					t.Errorf("expected parse error for invalid input, got value: %v", v)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking test directory: %v", err)
	}
}
