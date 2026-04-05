package bubbletea

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestParseKey(t *testing.T) {
	tests := []struct {
		input     string
		wantCode  rune
		wantText  string
		wantMod   tea.KeyMod
	}{
		{"enter", tea.KeyEnter, "", 0},
		{"space", tea.KeySpace, " ", 0},
		{"ctrl+c", 0, "c", tea.ModCtrl},
		{"alt+a", 0, "a", tea.ModAlt},
		{"[paste]", 0, "paste", 0},
		{"a", 'a', "a", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			k, _ := ParseKey(tt.input)
			assert.Equal(t, tt.wantCode, k.Code)
			if tt.wantText != "" {
				assert.Equal(t, tt.wantText, k.Text)
			}
			if tt.wantMod != 0 {
				assert.True(t, k.Mod.Contains(tt.wantMod))
			}
		})
	}
}
