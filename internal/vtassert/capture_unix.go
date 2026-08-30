//go:build unix

package vtassert

import (
	"os"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// CaptureScreen reads PTY output and parses it through the VT parser
// to produce a structured screen snapshot with cell-level attributes.
func CaptureScreen(ptyFile *os.File, rows, cols int) *vt.Screen {
	vterm := vt.NewVTerm(rows, cols)
	buf := make([]byte, 4096)
	for {
		n, err := ptyFile.Read(buf)
		if n > 0 {
			vterm.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return vterm.ActiveScreen()
}
