package tokenizer

// TrieNode is a node in the byte-level prefix trie used by Unigram
// for common prefix search over the vocabulary.
type TrieNode struct {
	children map[byte]*TrieNode
	isLeaf   bool
}

// newTrieNode creates a new trie node.
func newTrieNode() *TrieNode {
	return &TrieNode{children: make(map[byte]*TrieNode)}
}

// Trie is a byte-level trie for vocabulary prefix search.
type Trie struct {
	root *TrieNode
}

// NewTrie creates an empty trie.
func NewTrie() *Trie {
	return &Trie{root: newTrieNode()}
}

// Insert adds a byte sequence to the trie, marking the final node as leaf.
func (t *Trie) Insert(bytes []byte) {
	node := t.root
	for _, b := range bytes {
		child, ok := node.children[b]
		if !ok {
			child = newTrieNode()
			node.children[b] = child
		}
		node = child
	}
	node.isLeaf = true
}

// CommonPrefixSearch yields all leaf-terminated prefixes found when
// iterating over the given byte sequence. Each yield is a copy of the
// accumulated byte slice.
func (t *Trie) CommonPrefixSearch(iter func(yield func(byte) bool)) func(yield func([]byte) bool) {
	return func(yield func([]byte) bool) {
		node := t.root
		prefix := make([]byte, 0, 32)
		iter(func(b byte) bool {
			prefix = append(prefix, b)
			child, ok := node.children[b]
			if !ok {
				return false // stop iteration
			}
			node = child
			if node.isLeaf {
				// Yield a copy of the prefix
				cp := make([]byte, len(prefix))
				copy(cp, prefix)
				if !yield(cp) {
					return false
				}
			}
			return true
		})
	}
}

// Clone returns a deep copy of the trie.
func (t *Trie) Clone() *Trie {
	var cloneNode func(n *TrieNode) *TrieNode
	cloneNode = func(n *TrieNode) *TrieNode {
		c := newTrieNode()
		c.isLeaf = n.isLeaf
		for k, v := range n.children {
			c.children[k] = cloneNode(v)
		}
		return c
	}
	return &Trie{root: cloneNode(t.root)}
}
