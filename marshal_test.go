package golangcijson_test

import (
	"strings"
	"testing"

	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/golangci/golangci-lint/v2/pkg/config"
)

// TestOSChdirFalseIsWritten is the case that started this design: os-chdir
// defaults to true in golangci-lint (see defaultLintersSettings in the
// vendored source), so an explicit false must be written, not treated as
// "unset" the way an ordinary zero value would be. Every other field of
// UseTestingSettings is set too, matching how internal/lintgen actually
// constructs such a literal — a field left off is not "keep its default", it
// is Go zero, so a caller wanting a true-default field to stay true always
// has to say so.
func TestOSChdirFalseIsWritten(t *testing.T) {
	cfg := config.Config{
		Linters: config.Linters{
			Settings: config.LintersSettings{
				UseTesting: config.UseTestingSettings{
					OSChdir:      false,
					OSMkdirTemp:  true,
					OSSetenv:     true,
					OSCreateTemp: true,
				},
			},
		},
	}

	got, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if want := `"os-chdir": false`; !strings.Contains(string(got), want) {
		t.Errorf("explicit %s was lost:\n%s", want, got)
	}

	if want := `"os-mkdir-temp": true,`; !strings.Contains(string(got), want) {
		t.Errorf("got:\n%s", got)
	}
}

// TestZeroSettingsStructOmitsTheWholeBlock documents a real, inherent limit
// of this approach, rather than a bug: a settings struct is itself omitted
// (via the omitzero on its own field in the parent, e.g.
// LintersSettings.UseTesting) when every one of its own fields happens to be
// the Go zero value — because a plain Go value has no way to represent "this
// key was present in the file but every field inside took the default",
// distinct from "this key was absent". golangci-lint's own mapstructure
// decode has exactly the same limitation for the same reason: both start
// from a plain struct value with no per-field "was this set" tracking.
//
// The practical consequence: a caller writing a settings literal must set
// every field whose true default it wants to keep, not just the ones it
// wants to change — leaving a field off is Go zero, never "default".
func TestZeroSettingsStructOmitsTheWholeBlock(t *testing.T) {
	cfg := config.Config{
		Linters: config.Linters{
			Settings: config.LintersSettings{
				UseTesting: config.UseTestingSettings{OSChdir: false},
			},
		},
	}

	got, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(got), "usetesting") {
		t.Errorf("expected the whole usetesting block to disappear (every field is Go zero), got:\n%s", got)
	}
}

// TestMarshalRealisticConfig pins down the exact YAML this repo's generator
// (internal/lintgen) depends on not shifting silently.
func TestMarshalRealisticConfig(t *testing.T) {
	cfg := config.Config{
		Version: "2",
		Linters: config.Linters{
			Default: config.GroupNone,
			Enable:  []string{"usetesting", "tagalign"},
			Settings: config.LintersSettings{
				UseTesting: config.UseTestingSettings{
					OSChdir:      true,
					OSMkdirTemp:  true,
					OSSetenv:     true,
					OSCreateTemp: true,
				},
				TagAlign: config.TagAlignSettings{
					Align: true,
					Sort:  true,
					Order: []string{"json", "required"},
				},
			},
		},
		Formatters: config.Formatters{
			Enable:     []string{"gofumpt"},
			Exclusions: config.FormatterExclusions{Generated: "strict"},
		},
	}

	got, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	want := `{
  "version": "2",
  "linters": {
    "default": "none",
    "enable": [
      "usetesting",
      "tagalign"
    ],
    "settings": {
      "tagalign": {
        "align": true,
        "sort": true,
        "order": [
          "json",
          "required"
        ]
      },
      "usetesting": {
        "os-chdir": true,
        "os-mkdir-temp": true,
        "os-setenv": true,
        "os-create-temp": true
      }
    }
  },
  "formatters": {
    "enable": [
      "gofumpt"
    ],
    "exclusions": {
      "generated": "strict"
    }
  }
}`

	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestMarshalWriteJSONHasNoTrailingNewline matches encoding/json.Marshal's
// convention (no trailing newline).
func TestMarshalWriteJSONHasNoTrailingNewline(t *testing.T) {
	got, err := golangcijson.Marshal(config.Config{Version: "2"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.HasSuffix(string(got), "\n") {
		t.Errorf("output has a trailing newline: %q", got)
	}
}

// TestMarshalInternalFieldsAreExcluded checks that config.Config's own
// InternalTest and InternalCmdTest — present for golangci-lint's use, not
// part of the on-disk schema, and carrying no mapstructure tag at all — never
// reach the output, however they are set.
func TestMarshalInternalFieldsAreExcluded(t *testing.T) {
	cfg := config.Config{
		Version:         "2",
		InternalTest:    true,
		InternalCmdTest: true,
	}

	got, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(got), "nternal") {
		t.Errorf("an internal field leaked into the output: %s", got)
	}
}

// TestMarshalFormatterExclusions is the original bug this package traces
// back to: FormatterExclusions marshaling with capitalised Go field names and
// every zero value spelled out, because the old shadow-type registry had
// missed it. It is comprehensively covered here, but this specific case is
// worth keeping as a named regression test.
func TestMarshalFormatterExclusions(t *testing.T) {
	cfg := config.Config{
		Formatters: config.Formatters{
			Exclusions: config.FormatterExclusions{
				Generated:  "strict",
				Paths:      []string{"vendor/.*"},
				WarnUnused: true,
			},
		},
	}

	got, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`"generated": "strict",`,
		`"paths":`,
		`"vendor/.*"`,
		`"warn-unused": true`,
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("output is missing %q:\n%s", want, got)
		}
	}

	for _, bad := range []string{"Generated", "Paths", "WarnUnused"} {
		if strings.Contains(string(got), bad) {
			t.Errorf("output leaked an untranslated Go field name %q:\n%s", bad, got)
		}
	}
}

// TestMarshalSquashedTypes exercises every type that embeds another via
// mapstructure:",squash" (see squash.go): the embedded value's fields and the
// type's own fields must both appear, flattened into one object rather than
// nested.
func TestMarshalSquashedTypes(t *testing.T) {
	t.Run("ExcludeRule contributes only its embedded BaseRule", func(t *testing.T) {
		cfg := config.Config{
			Linters: config.Linters{
				Exclusions: config.LinterExclusions{
					Rules: []config.ExcludeRule{{
						Path: "vendor/.*", Linters: []string{"errcheck"},
					}},
				},
			},
		}

		got, err := golangcijson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{`"path": "vendor/.*"`, `"errcheck"`} {
			if !strings.Contains(string(got), want) {
				t.Errorf("got:\n%s\nmissing %q", got, want)
			}
		}
	})

	t.Run("SeverityRule merges BaseRule with its own Severity field", func(t *testing.T) {
		cfg := config.Config{
			Severity: config.Severity{
				Rules: []config.SeverityRule{{
					Path:     "foo.go",
					Severity: "error",
				}},
			},
		}

		got, err := golangcijson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{`"path": "foo.go",`, `"severity": "error"`} {
			if !strings.Contains(string(got), want) {
				t.Errorf("got:\n%s\nmissing %q", got, want)
			}
		}
	})

	t.Run("GoModGuardv2Blocked merges GoModGuardv2Base with Recommendations and Reason", func(t *testing.T) {
		cfg := config.Config{
			Linters: config.Linters{
				Settings: config.LintersSettings{
					Gomodguardv2: config.GoModGuardv2Settings{
						Blocked: []config.GoModGuardv2Blocked{{
							Module: "bad/module",
							Reason: "deprecated",
						}},
					},
				},
			},
		}

		got, err := golangcijson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{`"module": "bad/module",`, `"reason": "deprecated"`} {
			if !strings.Contains(string(got), want) {
				t.Errorf("got:\n%s\nmissing %q", got, want)
			}
		}
	})

	t.Run("TagliatelleCase merges TagliatelleBase with Overrides, which itself squashes", func(t *testing.T) {
		cfg := config.Config{
			Linters: config.Linters{
				Settings: config.LintersSettings{
					Tagliatelle: config.TagliatelleSettings{
						Case: config.TagliatelleCase{
							TagliatelleBase: config.TagliatelleBase{UseFieldName: true},
							Overrides: []config.TagliatelleOverrides{{
								UseFieldName: false,
								Package:      "foo/bar",
							}},
						},
					},
				},
			},
		}

		got, err := golangcijson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{`"use-field-name": true,`, `"pkg": "foo/bar"`} {
			if !strings.Contains(string(got), want) {
				t.Errorf("got:\n%s\nmissing %q", got, want)
			}
		}
	})

	t.Run("output formats: Text, Tab and JUnitXML each merge SimpleFormat with their own fields", func(t *testing.T) {
		cfg := config.Config{
			Output: config.Output{
				Formats: config.Formats{
					Text:     config.Text{SimpleFormat: config.SimpleFormat{Path: "stdout"}, Colors: true},
					Tab:      config.Tab{SimpleFormat: config.SimpleFormat{Path: "tab.txt"}, PrintLinterName: true},
					JUnitXML: config.JUnitXML{SimpleFormat: config.SimpleFormat{Path: "out.xml"}, Extended: true},
				},
			},
		}

		got, err := golangcijson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{
			`"path": "stdout",`, `"colors": true`,
			`"path": "tab.txt",`, `"print-linter-name": true`,
			`"path": "out.xml",`, `"extended": true`,
		} {
			if !strings.Contains(string(got), want) {
				t.Errorf("got:\n%s\nmissing %q", got, want)
			}
		}
	})
}
