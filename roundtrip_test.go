package golangcijson_test

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/golangci/golangci-lint/v2/pkg/config"
	"github.com/spf13/viper"
)

// filler sets every field golangci-lint decodes to a distinct non-zero value,
// except the leaf numbered skip, which it leaves at its Go zero value.
type filler struct {
	n, skip int
	// emptied reports that skipping left the enclosing struct entirely zero.
	emptied bool
}

// leaf numbers the next leaf and reports whether to fill it.
func (f *filler) leaf() bool {
	f.n++
	return f.n != f.skip
}

func (f *filler) fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			sf := v.Type().Field(i)
			if !sf.IsExported() {
				continue
			}

			// golangci-lint never decodes these, so they must never be written.
			if tag, ok := sf.Tag.Lookup("mapstructure"); !ok || tag == "-" {
				continue
			}

			f.fill(v.Field(i))
		}

		if !f.emptied && f.n >= f.skip && v.IsZero() {
			f.emptied = true
		}
	case reflect.Pointer:
		if f.leaf() {
			v.Set(reflect.New(v.Type().Elem()))
			f.fill(v.Elem())
		}
	case reflect.Slice:
		if f.leaf() {
			v.Set(reflect.MakeSlice(v.Type(), 1, 1))
			f.fill(v.Index(0))
		}
	case reflect.Map:
		if f.leaf() {
			f.fillMap(v)
		}
	case reflect.Interface:
		if f.leaf() {
			v.Set(reflect.ValueOf(fmt.Sprintf("any%d", f.n)))
		}
	default:
		if !f.leaf() {
			return
		}

		switch {
		case v.Type() == reflect.TypeFor[time.Duration]():
			v.SetInt(int64(f.n) * int64(time.Second))
		case v.Kind() == reflect.Bool:
			v.SetBool(true)
		case v.Kind() == reflect.String:
			v.SetString(fmt.Sprintf("s%d", f.n))
		case v.CanInt():
			v.SetInt(int64(f.n))
		case v.CanUint():
			v.SetUint(uint64(f.n))
		case v.CanFloat():
			v.SetFloat(float64(f.n) + 0.5)
		default:
			panic("unhandled kind " + v.Kind().String())
		}
	}
}

func (f *filler) fillMap(v reflect.Value) {
	v.Set(reflect.MakeMap(v.Type()))
	key := reflect.ValueOf(fmt.Sprintf("k%d", f.n)).Convert(v.Type().Key())
	elem := reflect.New(v.Type().Elem()).Elem()

	// A map entry whose value is null is dropped on reading, so never leave one nil.
	switch elem.Kind() {
	case reflect.Pointer:
		elem.Set(reflect.New(elem.Type().Elem()))
		f.fill(elem.Elem())
	case reflect.Map:
		f.fillMap(elem)
	case reflect.Interface:
		elem.Set(reflect.ValueOf(fmt.Sprintf("any%d", f.n)))
	default:
		f.fill(elem)
	}

	v.SetMapIndex(key, elem)
}

func filledConfig(skip int) (cfg config.Config, leaves int, emptied bool) {
	f := &filler{skip: skip}
	f.fill(reflect.ValueOf(&cfg).Elem())

	return cfg, f.n, f.emptied
}

// decode reads b the way golangci-lint reads a config file: through viper,
// with golangci-lint's own decode hooks, on top of its own defaults.
func decode(t *testing.T, b []byte) config.Config {
	t.Helper()

	v := viper.New()
	v.SetConfigType("json")

	if err := v.ReadConfig(bytes.NewReader(b)); err != nil {
		t.Fatalf("output is not a readable config: %v\n%s", err, b)
	}

	// NewDefault shares its slices and maps with package-level defaults, which decoding would overwrite.
	cfg := deepCopy(reflect.ValueOf(config.NewDefault())).Interface().(*config.Config)
	if err := v.Unmarshal(cfg, viper.DecodeHook(config.DecodeHookFunc())); err != nil {
		t.Fatalf("golangci-lint cannot decode the output: %v\n%s", err, b)
	}

	return *cfg
}

func deepCopy(v reflect.Value) reflect.Value {
	c := reflect.New(v.Type()).Elem()

	switch v.Kind() {
	case reflect.Struct:
		c.Set(v)

		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				c.Field(i).Set(deepCopy(v.Field(i)))
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			c.Set(reflect.New(v.Type().Elem()))
			c.Elem().Set(deepCopy(v.Elem()))
		}
	case reflect.Slice:
		if !v.IsNil() {
			c.Set(reflect.MakeSlice(v.Type(), v.Len(), v.Len()))

			for i := range v.Len() {
				c.Index(i).Set(deepCopy(v.Index(i)))
			}
		}
	case reflect.Map:
		if !v.IsNil() {
			c.Set(reflect.MakeMap(v.Type()))

			for iter := v.MapRange(); iter.Next(); {
				c.SetMapIndex(iter.Key(), deepCopy(iter.Value()))
			}
		}
	default:
		c.Set(v)
	}

	return c
}

// normalize treats an empty slice or map like a nil one, since neither is written.
func normalize(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				normalize(v.Field(i))
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			normalize(v.Elem())
		}
	case reflect.Slice:
		if v.Len() == 0 {
			v.SetZero()
		}

		for i := range v.Len() {
			normalize(v.Index(i))
		}
	case reflect.Map:
		if v.Len() == 0 {
			v.SetZero()
		}

		for iter := v.MapRange(); iter.Next(); {
			elem := reflect.New(iter.Value().Type()).Elem()
			elem.Set(iter.Value())
			normalize(elem)
			v.SetMapIndex(iter.Key(), elem)
		}
	default:
	}
}

func assertRoundTrip(t *testing.T, want config.Config) {
	t.Helper()

	b, err := golangcijson.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	got := decode(t, b)
	// golangci-lint copies this from formatters.settings rather than decoding it.
	got.Linters.Settings.FormatterSettings = config.FormatterSettings{}
	want.Linters.Settings.FormatterSettings = config.FormatterSettings{}

	normalize(reflect.ValueOf(&want).Elem())
	normalize(reflect.ValueOf(&got).Elem())

	if !reflect.DeepEqual(got, want) {
		t.Errorf("golangci-lint reads back a different config:\ngot:  %+v\nwant: %+v\njson:\n%s", got, want, b)
	}
}

// TestRoundTripEveryField fills every field of every type reachable from
// config.Config, so a JSON key that differs from its mapstructure tag, or a
// field that is dropped, makes golangci-lint read back something else.
func TestRoundTripEveryField(t *testing.T) {
	cfg, leaves, _ := filledConfig(0)
	if leaves < 650 {
		t.Fatalf("filled only %d leaves", leaves)
	}

	assertRoundTrip(t, cfg)
}

// TestRoundTripEachZeroField leaves one field at a time at its Go zero value:
// the output must say so, either by omitting the key where golangci-lint's
// default is that zero value, or by writing it where the default differs.
func TestRoundTripEachZeroField(t *testing.T) {
	_, leaves, _ := filledConfig(0)

	for skip := 1; skip <= leaves; skip++ {
		cfg, _, emptied := filledConfig(skip)
		if emptied {
			continue // the whole block is omitted: see TestZeroSettingsStructOmitsTheWholeBlock
		}

		t.Run(fmt.Sprint(skip), func(t *testing.T) { assertRoundTrip(t, cfg) })
	}
}

// TestRoundTripExplicitDefaults checks that golangci-lint's own defaults
// survive a round trip, including those that are true or non-zero.
func TestRoundTripExplicitDefaults(t *testing.T) {
	assertRoundTrip(t, *config.NewDefault())
}

// TestMarshalWriteMatchesMarshal checks both entry points produce identical output.
func TestMarshalWriteMatchesMarshal(t *testing.T) {
	cfg, _, _ := filledConfig(0)

	want, err := golangcijson.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := golangcijson.MarshalWrite(&buf, cfg); err != nil {
		t.Fatal(err)
	}

	if buf.String() != string(want) {
		t.Errorf("MarshalWrite:\n%s\nMarshal:\n%s", buf.String(), want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("disk full") }

func TestMarshalWriteReportsWriterError(t *testing.T) {
	err := golangcijson.MarshalWrite(failingWriter{}, config.Config{Version: "2"})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("want the writer's error, got %v", err)
	}
}

// TestMarshalReportsUnencodableSettings checks that a free-form settings value
// JSON cannot represent is reported, not dropped or panicked over.
func TestMarshalReportsUnencodableSettings(t *testing.T) {
	cfg := config.Config{
		Linters: config.Linters{
			Settings: config.LintersSettings{
				Custom: map[string]config.CustomLinterSettings{
					"mine": {Path: "mine.so", Settings: make(chan int)},
				},
			},
		},
	}

	if _, err := golangcijson.Marshal(cfg); err == nil {
		t.Error("Marshal: want an error for a channel in custom settings")
	}

	if err := golangcijson.MarshalWrite(io.Discard, cfg); err == nil {
		t.Error("MarshalWrite: want an error for a channel in custom settings")
	}
}

// TestMarshalTimeoutIsReadable checks run.timeout is written the way a person writes it in a config file.
func TestMarshalTimeoutIsReadable(t *testing.T) {
	got, err := golangcijson.Marshal(config.Config{Run: config.Run{Timeout: 90 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}

	if want := `"timeout": "1m30s"`; !strings.Contains(string(got), want) {
		t.Errorf("want %s in:\n%s", want, got)
	}
}
