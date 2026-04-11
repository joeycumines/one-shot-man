package scripting

import (
	"os"

	"github.com/joeycumines/go-prompt"
)

// goPromptSyncOptions returns go-prompt options enabling the sync protocol,
// when activated via the OSM_SYNC_PROTOCOL=1 environment variable. E2E tests
// set this variable when building the osm binary to get deterministic I/O.
func goPromptSyncOptions() []prompt.Option {
	if os.Getenv("OSM_SYNC_PROTOCOL") == "1" {
		return []prompt.Option{prompt.WithSyncProtocol(true)}
	}
	return nil
}
