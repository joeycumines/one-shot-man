// Package ptyio provides buffered, backpressure-aware I/O primitives for
// PTY file descriptors.
//
// It includes a BufferedReader that runs a blocking read loop with
// channel-based output (providing natural backpressure when the consumer
// pauses), a BufferedWriter that handles partial writes and EAGAIN retry,
// and platform-abstracted interfaces for terminal state management and
// non-blocking flag control.
package ptyio
