package vt

import "io"

// Compile-time interface checks (T097).
var _ io.Writer = (*VTerm)(nil)
