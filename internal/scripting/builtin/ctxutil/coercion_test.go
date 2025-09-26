package ctxutil

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

func TestGojaCoercion(t *testing.T) {
	runtime := goja.New()

	t.Run("ToObject", func(t *testing.T) {
		// null -> error
		v, err := runtime.RunString("null")
		require.NoError(t, err)
		obj, err := toObject(runtime, v)
		require.Error(t, err)
		require.Nil(t, obj)
		require.Contains(t, err.Error(), "TypeError: Cannot convert undefined or null to object")

		// undefined -> error
		v, err = runtime.RunString("undefined")
		require.NoError(t, err)
		obj, err = toObject(runtime, v)
		require.Error(t, err)
		require.Nil(t, obj)
		require.Contains(t, err.Error(), "TypeError: Cannot convert undefined or null to object")

		// string -> String object
		v, err = runtime.RunString("'hello'")
		require.NoError(t, err)
		obj, err = toObject(runtime, v)
		require.NoError(t, err)
		require.NotNil(t, obj)
		require.Equal(t, "String", obj.ClassName())

		// number -> Number object
		v, err = runtime.RunString("123")
		require.NoError(t, err)
		obj, err = toObject(runtime, v)
		require.NoError(t, err)
		require.NotNil(t, obj)
		require.Equal(t, "Number", obj.ClassName())

		// boolean -> Boolean object
		v, err = runtime.RunString("true")
		require.NoError(t, err)
		obj, err = toObject(runtime, v)
		require.NoError(t, err)
		require.NotNil(t, obj)
		require.Equal(t, "Boolean", obj.ClassName())
	})

	t.Run("ToString", func(t *testing.T) {
		// null -> "null"
		v, err := runtime.RunString("null")
		require.NoError(t, err)
		require.Equal(t, "null", v.String())

		// undefined -> "undefined"
		v, err = runtime.RunString("undefined")
		require.NoError(t, err)
		require.Equal(t, "undefined", v.String())

		// object -> "[object Object]"
		v, err = runtime.RunString("({})")
		require.NoError(t, err)
		require.Equal(t, "[object Object]", v.String())
	})

	t.Run("ToInteger", func(t *testing.T) {
		// null -> 0
		v, err := runtime.RunString("null")
		require.NoError(t, err)
		require.Equal(t, int64(0), v.ToInteger())

		// undefined -> 0
		v, err = runtime.RunString("undefined")
		require.NoError(t, err)
		require.Equal(t, int64(0), v.ToInteger())

		// "123" -> 123
		v, err = runtime.RunString("'123'")
		require.NoError(t, err)
		require.Equal(t, int64(123), v.ToInteger())

		// "abc" -> 0
		v, err = runtime.RunString("'abc'")
		require.NoError(t, err)
		require.Equal(t, int64(0), v.ToInteger())
	})

	t.Run("Export", func(t *testing.T) {
		// null -> nil interface{}
		v, err := runtime.RunString("null")
		require.NoError(t, err)
		var i interface{}
		err = runtime.ExportTo(v, &i)
		require.NoError(t, err)
		require.Nil(t, i)

		// undefined -> nil interface{}
		v, err = runtime.RunString("undefined")
		require.NoError(t, err)
		err = runtime.ExportTo(v, &i)
		require.NoError(t, err)
		require.Nil(t, i)

		// array -> []interface{}
		v, err = runtime.RunString("[1, 'a', true]")
		require.NoError(t, err)
		err = runtime.ExportTo(v, &i)
		require.NoError(t, err)
		s, ok := i.([]interface{})
		require.True(t, ok)
		require.Len(t, s, 3)
		require.Equal(t, int64(1), s[0])
		require.Equal(t, "a", s[1])
		require.Equal(t, true, s[2])
	})
}
