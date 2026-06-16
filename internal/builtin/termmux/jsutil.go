package termmux

import "github.com/dop251/goja"

// jsGetString returns the string value for key from obj, or defaultVal if the
// key is missing, undefined, or null.
func jsGetString(obj *goja.Object, key, defaultVal string) string {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return defaultVal
	}
	return v.String()
}

// jsGetInt returns the integer value for key from obj, or defaultVal if the
// key is missing, undefined, or null.
func jsGetInt(obj *goja.Object, key string, defaultVal int) int {
	v := obj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return defaultVal
	}
	return int(v.ToInteger())
}
