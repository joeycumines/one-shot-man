package pabt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewAction_NilNode_Panics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		NewAction("testAction", nil, nil, nil)
	}, "NewAction should panic on nil node")
}

func TestNewAction_PanicMessage(t *testing.T) {
	t.Parallel()

	require.PanicsWithValue(t, "pabt.NewAction: node cannot be nil (action=\"testAction\")", func() {
		NewAction("testAction", nil, nil, nil)
	})
}
