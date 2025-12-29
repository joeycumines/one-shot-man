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
		m       map[string]interface{}
		key     string
		def     int
		want    int
		wantErr bool
	}{
		{"missing", map[string]interface{}{}, "missing", 7, 7, false},
		{"undefined", map[string]interface{}{"i": undef}, "i", 8, 8, false},
		{"nil", map[string]interface{}{"i": nil}, "i", 9, 9, false},
		{"int", map[string]interface{}{"i": int(2)}, "i", 0, 2, false},
		{"int32", map[string]interface{}{"i": int32(3)}, "i", 0, 3, false},
		{"int64", map[string]interface{}{"i": int64(4)}, "i", 0, 4, false},
		{"float64", map[string]interface{}{"i": float64(3.9)}, "i", 0, 3, false},
		{"float32", map[string]interface{}{"i": float32(6.1)}, "i", 0, 6, false},
		{"bad", map[string]interface{}{"i": "nope"}, "i", 0, 0, true},
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
		m       map[string]interface{}
		key     string
		def     string
		want    string
		wantErr bool
	}{
		{"missing", map[string]interface{}{}, "missing", "d", "d", false},
		{"undefined", map[string]interface{}{"s": undef}, "s", "d", "d", false},
		{"nil", map[string]interface{}{"s": nil}, "s", "d", "d", false},
		{"ok", map[string]interface{}{"s": "hello"}, "s", "", "hello", false},
		{"bad", map[string]interface{}{"s": 123}, "s", "", "", true},
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
		m       map[string]interface{}
		key     string
		def     bool
		want    bool
		wantErr bool
	}{
		{"missing", map[string]interface{}{}, "missing", true, true, false},
		{"undefined", map[string]interface{}{"b": undef}, "b", false, false, false},
		{"nil", map[string]interface{}{"b": nil}, "b", true, true, false},
		{"true", map[string]interface{}{"b": true}, "b", false, true, false},
		{"false", map[string]interface{}{"b": false}, "b", true, false, false},
		{"bad", map[string]interface{}{"b": "nope"}, "b", false, false, true},
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
		m       map[string]interface{}
		key     string
		want    []string
		wantErr bool
	}{
		{"missing", map[string]interface{}{}, "missing", nil, false},
		{"undefined", map[string]interface{}{"arr": undef}, "arr", nil, false},
		{"nil", map[string]interface{}{"arr": nil}, "arr", nil, false},
		{"good", map[string]interface{}{"arr": []interface{}{"a", "b"}}, "arr", []string{"a", "b"}, false},
		{"empty", map[string]interface{}{"arr": []interface{}{}}, "arr", []string{}, false},
		{"badElem", map[string]interface{}{"arr": []interface{}{"ok", 5}}, "arr", nil, true},
		{"notArray", map[string]interface{}{"arr": "no"}, "arr", nil, true},
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
