package compositor

import (
	"regexp"
	"strings"
)

// Bubblezone marks a region with the private CSI sequence ESC [ <digits> z,
// which the bubblezone scanner recognises and strips after measuring. The
// ultraviolet cell parser behind lipgloss.Canvas deliberately drops CSI
// sequences that are not SGR, because replaying a cursor-moving sequence from
// a cell would desynchronise the terminal. That would silently lose every zone
// boundary crossing the compositor.
//
// APC sequences are the one zero-width form the parser carries through a cell,
// so the compositor rewrites zone markers into APC carriers before building
// layers and rewrites them back after the canvas render. The carrier has zero
// display width, so cell positions and widths are unchanged.
//
// The transcoder accepts exactly the bubblezone marker grammar; every other
// escape sequence is left alone.
const (
	zoneCarrierPrefix = "\x1b_osm-zone:"
	zoneCarrierSuffix = "\x1b\\"
)

var (
	zoneMarkerPattern  = regexp.MustCompile("\x1b\\[[0-9]+z")
	zoneCarrierPattern = regexp.MustCompile("\x1b_osm-zone:([0-9]+)\x1b\\\\")
)

// encodeZoneMarkers replaces bubblezone zone markers with APC carriers that
// survive the ultraviolet cell parser.
func encodeZoneMarkers(content string) string {
	if !strings.Contains(content, "\x1b[") {
		return content
	}
	return zoneMarkerPattern.ReplaceAllStringFunc(content, func(marker string) string {
		return zoneCarrierPrefix + marker[2:len(marker)-1] + zoneCarrierSuffix
	})
}

// decodeZoneMarkers restores the APC carriers produced by encodeZoneMarkers to
// the bubblezone markers consumers scan for.
func decodeZoneMarkers(rendered string) string {
	if !strings.Contains(rendered, zoneCarrierPrefix) {
		return rendered
	}
	return zoneCarrierPattern.ReplaceAllString(rendered, "\x1b[${1}z")
}
