package json5

import (
	"os"
	"path/filepath"
	"testing"
)

// knownFailures lists test files where the parser does not yet match the spec.
// Each entry documents the non-conformance so it can be fixed later.
var knownFailures = map[string]string{}

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
