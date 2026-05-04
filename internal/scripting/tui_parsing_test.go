package scripting

import (
	"reflect"
	"testing"

	"github.com/dop251/goja"
)

func TestGetIntTable(t *testing.T) {
	undef := goja.Undefined()
	cases := []struct {
		name    string
		m       map[string]any
		key     string
		def     int
		want    int
		wantErr bool
	}{
		{"missing", map[string]any{}, "missing", 7, 7, false},
		{"undefined", map[string]any{"i": undef}, "i", 8, 8, false},
		{"nil", map[string]any{"i": nil}, "i", 9, 9, false},
		{"int", map[string]any{"i": int(2)}, "i", 0, 2, false},
		{"int32", map[string]any{"i": int32(3)}, "i", 0, 3, false},
		{"int64", map[string]any{"i": int64(4)}, "i", 0, 4, false},
		{"float64", map[string]any{"i": float64(3.9)}, "i", 0, 3, false},
		{"float32", map[string]any{"i": float32(6.1)}, "i", 0, 6, false},
		{"bad", map[string]any{"i": "nope"}, "i", 0, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := getInt(tc.m, tc.key, tc.def)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if err == nil && v != tc.want {
				t.Fatalf("expected %d got %d", tc.want, v)
			}
		})
	}
}

func TestGetStringTable(t *testing.T) {
	undef := goja.Undefined()
	cases := []struct {
		name    string
		m       map[string]any
		key     string
		def     string
		want    string
		wantErr bool
	}{
		{"missing", map[string]any{}, "missing", "d", "d", false},
		{"undefined", map[string]any{"s": undef}, "s", "d", "d", false},
		{"nil", map[string]any{"s": nil}, "s", "d", "d", false},
		{"ok", map[string]any{"s": "hello"}, "s", "", "hello", false},
		{"bad", map[string]any{"s": 123}, "s", "", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := getString(tc.m, tc.key, tc.def)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if err == nil && s != tc.want {
				t.Fatalf("expected %q got %q", tc.want, s)
			}
		})
	}
}

func TestGetBoolTable(t *testing.T) {
	undef := goja.Undefined()
	cases := []struct {
		name    string
		m       map[string]any
		key     string
		def     bool
		want    bool
		wantErr bool
	}{
		{"missing", map[string]any{}, "missing", true, true, false},
		{"undefined", map[string]any{"b": undef}, "b", false, false, false},
		{"nil", map[string]any{"b": nil}, "b", true, true, false},
		{"true", map[string]any{"b": true}, "b", false, true, false},
		{"false", map[string]any{"b": false}, "b", true, false, false},
		{"bad", map[string]any{"b": "nope"}, "b", false, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := getBool(tc.m, tc.key, tc.def)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if err == nil && b != tc.want {
				t.Fatalf("expected %v got %v", tc.want, b)
			}
		})
	}
}

func TestGetStringSliceTable(t *testing.T) {
	undef := goja.Undefined()
	cases := []struct {
		name    string
		m       map[string]any
		key     string
		want    []string
		wantErr bool
	}{
		{"missing", map[string]any{}, "missing", nil, false},
		{"undefined", map[string]any{"arr": undef}, "arr", nil, false},
		{"nil", map[string]any{"arr": nil}, "arr", nil, false},
		{"good", map[string]any{"arr": []any{"a", "b"}}, "arr", []string{"a", "b"}, false},
		{"empty", map[string]any{"arr": []any{}}, "arr", []string{}, false},
		{"badElem", map[string]any{"arr": []any{"ok", 5}}, "arr", nil, true},
		{"notArray", map[string]any{"arr": "no"}, "arr", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arr, err := getStringSlice(tc.m, tc.key)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if err == nil && !reflect.DeepEqual(arr, tc.want) {
				t.Fatalf("expected %#v got %#v", tc.want, arr)
			}
		})
	}
}

func TestGetFlagDefsTable(t *testing.T) {
	undef := goja.Undefined()
	cases := []struct {
		name    string
		m       map[string]any
		key     string
		want    []FlagDef
		wantErr bool
	}{
		{"missing", map[string]any{}, "flagDefs", nil, false},
		{"undefined", map[string]any{"flagDefs": undef}, "flagDefs", nil, false},
		{"nil", map[string]any{"flagDefs": nil}, "flagDefs", nil, false},
		{"one_flag", map[string]any{
			"flagDefs": []any{
				map[string]any{"name": "verbose", "description": "enable verbose output"},
			},
		}, "flagDefs", []FlagDef{{Name: "verbose", Description: "enable verbose output"}}, false},
		{"two_flags", map[string]any{
			"flagDefs": []any{
				map[string]any{"name": "out", "description": "output file"},
				map[string]any{"name": "fmt", "description": "format type"},
			},
		}, "flagDefs", []FlagDef{
			{Name: "out", Description: "output file"},
			{Name: "fmt", Description: "format type"},
		}, false},
		{"name_only", map[string]any{
			"flagDefs": []any{
				map[string]any{"name": "quiet"},
			},
		}, "flagDefs", []FlagDef{{Name: "quiet", Description: ""}}, false},
		{"skip_empty_name", map[string]any{
			"flagDefs": []any{
				map[string]any{"name": ""},
				map[string]any{"name": "keep"},
			},
		}, "flagDefs", []FlagDef{{Name: "keep", Description: ""}}, false},
		{"empty_array", map[string]any{
			"flagDefs": []any{},
		}, "flagDefs", nil, false},
		{"not_array", map[string]any{"flagDefs": "no"}, "flagDefs", nil, true},
		{"bad_element", map[string]any{
			"flagDefs": []any{"not_a_map"},
		}, "flagDefs", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defs, err := getFlagDefs(tc.m, tc.key)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error state: %v", err)
			}
			if err == nil && !reflect.DeepEqual(defs, tc.want) {
				t.Fatalf("expected %#v got %#v", tc.want, defs)
			}
		})
	}
}
