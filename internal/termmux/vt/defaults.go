package vt

// DefaultRows is the default terminal row count for VT operations.
const DefaultRows = 24

// DefaultCols is the default terminal column count for VT operations.
const DefaultCols = 80

// MaxScrollback is the maximum number of scrollback lines.
const MaxScrollback = 10000

// MaxProtocolLength is the maximum length of a DCS or OSC sequence buffer.
// Both DCS and OSC sequences are capped at this length to prevent
// unbounded buffer growth from malformed or hostile escape sequences.
const MaxProtocolLength = 4096
