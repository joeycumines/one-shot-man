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
//
// Only balanced pairs are emitted. bubblezone's scanner pairs markers by their
// exact id, treating each occurrence alternately as an opening and a close, and
// it computes a zone's end from the close; an odd marker therefore corrupts the
// regions after it, because a later zone's opening is read as an earlier zone's
// close. A carrier can lose its partner two ways: a layer clipped at the canvas
// edge drops the cell it rode in, and an overlay painting that cell replaces it.
// Either way the surviving carrier is dropped here rather than emitted alone, so
// the rendered frame is always well formed.
func decodeZoneMarkers(rendered string) string {
	if !strings.Contains(rendered, zoneCarrierPrefix) {
		return rendered
	}

	matches := zoneCarrierPattern.FindAllStringSubmatchIndex(rendered, -1)
	// Every carrier becomes a piece; text between carriers is its own piece. A
	// carrier's piece is emitted only once its partner with the same id exists,
	// so a carrier whose partner was clipped or painted over disappears instead
	// of being emitted alone.
	type piece struct {
		text string
		emit bool
	}
	pieces := make([]piece, 0, len(matches)*2+1)
	open := map[string]int{}
	last := 0
	for _, match := range matches {
		if match[0] > last {
			pieces = append(pieces, piece{text: rendered[last:match[0]], emit: true})
		}
		id := rendered[match[2]:match[3]]
		marker := "\x1b[" + id + "z"
		if opening, paired := open[id]; paired {
			pieces[opening].emit = true
			pieces = append(pieces, piece{text: marker, emit: true})
			delete(open, id)
		} else {
			open[id] = len(pieces)
			pieces = append(pieces, piece{text: marker})
		}
		last = match[1]
	}
	if last < len(rendered) {
		pieces = append(pieces, piece{text: rendered[last:], emit: true})
	}

	var builder strings.Builder
	builder.Grow(len(rendered))
	for _, entry := range pieces {
		if entry.emit {
			builder.WriteString(entry.text)
		}
	}
	return builder.String()
}
