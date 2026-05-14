package tokenizer

import (
	"unicode/utf8"
)

// LatticeNode represents a node in the Unigram tokenization lattice.
// It holds the vocabulary ID, position, byte length, score, and
// backtrace information for Viterbi decoding.
type LatticeNode struct {
	ID             int     // vocabulary ID
	NodeID         int     // unique identifier within the lattice
	Pos            int     // byte position in the sentence
	Length         int     // byte length of the token
	Score          float64 // token score from vocab
	Prev           *LatticeNode
	BacktraceScore float64
}

// Lattice represents the tokenization search space for Unigram.
// It stores nodes indexed by their begin position in the sentence.
type Lattice struct {
	Sentence   string
	Len        int // byte length of sentence
	Nodes      []*LatticeNode
	BeginNodes [][]*LatticeNode // indexed by byte position
	EndNodes   [][]*LatticeNode // indexed by byte position
	BOSID      int
	EOSID      int
}

// NewLattice creates a lattice for a sentence with bos and eos nodes.
func NewLattice(sentence string, bosID, eosID int) *Lattice {
	slen := len(sentence)
	const reservedNodeSize = 16
	nodes := make([]*LatticeNode, 0, reservedNodeSize)
	beginNodes := make([][]*LatticeNode, slen+1)
	endNodes := make([][]*LatticeNode, slen+1)

	bos := &LatticeNode{ID: bosID, NodeID: 0, Pos: 0, Length: 0, Score: 0.0}
	eos := &LatticeNode{ID: eosID, NodeID: 1, Pos: slen, Length: 0, Score: 0.0}

	beginNodes[slen] = append(beginNodes[slen], eos)
	endNodes[0] = append(endNodes[0], bos)

	nodes = append(nodes, bos, eos)

	return &Lattice{
		Sentence:   sentence,
		Len:        slen,
		Nodes:      nodes,
		BeginNodes: beginNodes,
		EndNodes:   endNodes,
		BOSID:      bosID,
		EOSID:      eosID,
	}
}

// Insert adds a token node to the lattice at the given position with
// the given length, score, and vocab ID.
func (l *Lattice) Insert(pos, length int, score float64, id int) {
	nodeID := len(l.Nodes)
	node := &LatticeNode{
		ID:     id,
		NodeID: nodeID,
		Pos:    pos,
		Length: length,
		Score:  score,
	}

	l.BeginNodes[pos] = append(l.BeginNodes[pos], node)
	l.EndNodes[pos+length] = append(l.EndNodes[pos+length], node)
	l.Nodes = append(l.Nodes, node)
}

// Piece returns the token string for a node.
func (l *Lattice) Piece(node *LatticeNode) string {
	return l.Sentence[node.Pos : node.Pos+node.Length]
}

// Viterbi computes the best path through the lattice and returns it
// as a slice of nodes (excluding bos/eos).
func (l *Lattice) Viterbi() []*LatticeNode {
	pos := 0
	for pos <= l.Len {
		if len(l.BeginNodes[pos]) == 0 {
			return nil
		}
		for _, rnode := range l.BeginNodes[pos] {
			rnode.Prev = nil
			var bestScore float64
			var bestNode *LatticeNode
			for _, lnode := range l.EndNodes[pos] {
				score := lnode.BacktraceScore + rnode.Score
				if bestNode == nil || score > bestScore {
					bestNode = lnode
					bestScore = score
				}
			}
			if bestNode == nil {
				return nil
			}
			rnode.Prev = bestNode
			rnode.BacktraceScore = bestScore
		}
		// Advance to next char
		if pos >= l.Len {
			break
		}
		// Advance by one utf8 character
		_, size := utf8.DecodeRuneInString(l.Sentence[pos:])
		pos += size
	}

	// Backtrack from eos
	root := l.BeginNodes[l.Len][0]
	if root.Prev == nil {
		return nil
	}
	node := root.Prev
	results := make([]*LatticeNode, 0)
	for node.Prev != nil {
		results = append(results, node)
		node = node.Prev
	}
	// Reverse results
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return results
}

// NBest computes the top n tokenization paths (sorted by score, best first).
func (l *Lattice) NBest(n int) [][]*LatticeNode {
	if n == 0 {
		return nil
	}
	if n == 1 {
		path := l.Viterbi()
		if path == nil {
			return nil
		}
		return [][]*LatticeNode{path}
	}

	// We use a simple priority queue approach: maintain top n paths
	type pathEntry struct {
		nodes []*LatticeNode
		score float64
	}

	// First, fill backtrace scores (Viterbi pass)
	l.Viterbi()

	bos := l.EndNodes[0][0]
	eos := l.BeginNodes[l.Len][0]

	var paths []pathEntry

	// Collect paths using DFS with score tracking (excluding bos/eos)
	var collect func(node *LatticeNode, current []*LatticeNode, scoreSum float64)
	collect = func(node *LatticeNode, current []*LatticeNode, scoreSum float64) {
		if node == bos {
			// Reverse current to get forward order, then drop bos
			n := len(current)
			forward := make([]*LatticeNode, 0, n)
			for i := n - 1; i >= 0; i-- {
				if current[i] == bos {
					continue
				}
				forward = append(forward, current[i])
			}
			paths = append(paths, pathEntry{forward, scoreSum})
			return
		}
		for _, lnode := range l.EndNodes[node.Pos] {
			if lnode == node {
				continue // avoid self-loop (shouldn't happen)
			}
			newCurrent := make([]*LatticeNode, len(current)+1)
			copy(newCurrent[:len(current)], current)
			newCurrent[len(current)] = lnode
			collect(lnode, newCurrent, scoreSum+lnode.Score)
			if len(paths) >= n*10 {
				return // prune for performance
			}
		}
	}

	collect(eos, nil, 0)

	// Sort paths by score descending
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if paths[j].score > paths[i].score {
				paths[i], paths[j] = paths[j], paths[i]
			}
		}
	}

	result := make([][]*LatticeNode, 0, n)
	for i := 0; i < len(paths) && i < n; i++ {
		result = append(result, paths[i].nodes)
	}
	return result
}

// Tokens returns the best-path token strings.
func (l *Lattice) Tokens() []string {
	path := l.Viterbi()
	if path == nil {
		return nil
	}
	tokens := make([]string, len(path))
	for i, node := range path {
		tokens[i] = l.Piece(node)
	}
	return tokens
}
