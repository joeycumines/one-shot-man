// Package ptyio provides buffered, backpressure-aware I/O primitives for
// PTY file descriptors.
//
// It includes a BufferedReader that runs a blocking read loop with
// channel-based output (providing natural backpressure when the consumer
// pauses), and platform-abstracted interfaces for terminal state management
// and non-blocking flag control.
package ptyio
