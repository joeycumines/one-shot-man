package tokenizer

import "math"

// logSumExp returns log(exp(x) + exp(y)).
// If initMode is true, returns log(exp(y)) == y.
// This matches the Rust function log_sum_exp in lattice.rs.
//
// Moved from lattice.go: this helper is only used by test code.
// Keeping it in a _test.go file excludes it from the production binary
// and satisfies the deadcode checker per AGENTS.md policy.
func logSumExp(x, y float64, initMode bool) float64 {
	if initMode {
		return y
	}
	vmin, vmax := x, y
	if x > y {
		vmin, vmax = y, x
	}
	const kMinusLogEpsilon = 50.0
	if vmax > vmin+kMinusLogEpsilon {
		return vmax
	}
	return vmax + math.Log(math.Exp(vmin-vmax)+1.0)
}

// nBestTokens returns the top n token strings.
//
// Moved from lattice.go: this helper is only used by test code.
// Keeping it in a _test.go file excludes it from the production binary
// and satisfies the deadcode checker per AGENTS.md policy.
func (l *Lattice) nBestTokens(n int) [][]string {
	paths := l.NBest(n)
	if paths == nil {
		return nil
	}
	result := make([][]string, len(paths))
	for i, path := range paths {
		tokens := make([]string, len(path))
		for j, node := range path {
			tokens[j] = l.Piece(node)
		}
		result[i] = tokens
	}
	return result
}
