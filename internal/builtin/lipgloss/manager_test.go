package lipgloss

import (
	"os"
	"testing"

	"github.com/joeycumines/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	manager, err := NewManager()
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.True(t, *manager.detectedDarkBackground)

	// Options testing
	mFiles, err := NewManager(WithFiles(nil, nil))
	require.NoError(t, err)
	assert.True(t, *mFiles.detectedDarkBackground)

	_, err = NewManager(nil)
	assert.Error(t, err)
}

func TestAdaptiveBackgroundVariants(t *testing.T) {
	for _, tc := range []struct {
		name string
		dark bool
	}{
		{name: "dark", dark: true},
		{name: "light", dark: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{detectedDarkBackground: &tc.dark}
			vm := goja.New()
			module := vm.NewObject()
			require.NoError(t, module.Set("exports", vm.NewObject()))
			Require(manager)(vm, module)

			exports := module.Get("exports").ToObject(vm)
			darkFn, ok := goja.AssertFunction(exports.Get("hasDarkBackground"))
			require.True(t, ok)
			dark, err := darkFn(goja.Undefined())
			require.NoError(t, err)
			assert.Equal(t, tc.dark, dark.ToBoolean())

			lightFn, ok := goja.AssertFunction(exports.Get("hasLightBackground"))
			require.True(t, ok)
			light, err := lightFn(goja.Undefined())
			require.NoError(t, err)
			assert.Equal(t, !tc.dark, light.ToBoolean())
		})
	}
}

func TestCanQueryTerminalBackground(t *testing.T) {
	// Nil inputs must never query.
	assert.False(t, canQueryTerminalBackground(nil, nil))
	assert.False(t, canQueryTerminalBackground(os.Stdin, nil))
	assert.False(t, canQueryTerminalBackground(nil, os.Stdout))

	// In test execution, querying the terminal is always disabled to avoid hanging
	// on platforms like Windows or timing out waiting for OSC 11 responses.
	assert.False(t, canQueryTerminalBackground(os.Stdin, os.Stdout))
}
