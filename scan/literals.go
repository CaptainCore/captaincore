package scan

import "sort"

// literalIndex finds every one of a set of literals in a byte slice in a
// single pass (Aho-Corasick). Rule prefilters and literal-only patterns are
// the same few thousand strings for every file, so one pass over the file
// replaces one bytes.Contains per rule, and the cost of a rule set stops
// growing with its size.
type literalIndex struct {
	nodes []acNode
	root  [256]int32 // transitions out of the root, dense for speed
	n     int        // number of distinct literals
}

type acNode struct {
	keys []byte  // sorted transition bytes
	next []int32 // parallel to keys
	fail int32
	out  []int32 // literal ids that end at this node, own and via fail links
}

// newLiteralIndex builds the automaton. ids maps each distinct literal to its
// index in the hit slice that present fills.
func newLiteralIndex(lits [][]byte) (*literalIndex, map[string]int32) {
	li := &literalIndex{nodes: []acNode{{}}}
	ids := map[string]int32{}
	for _, lit := range lits {
		if len(lit) == 0 {
			continue
		}
		key := string(lit)
		if _, seen := ids[key]; seen {
			continue
		}
		id := int32(len(ids))
		ids[key] = id
		state := int32(0)
		for _, b := range lit {
			nxt := li.step(state, b)
			if nxt < 0 {
				li.nodes = append(li.nodes, acNode{})
				nxt = int32(len(li.nodes) - 1)
				n := &li.nodes[state]
				pos := sort.Search(len(n.keys), func(i int) bool { return n.keys[i] >= b })
				n.keys = append(n.keys, 0)
				n.next = append(n.next, 0)
				copy(n.keys[pos+1:], n.keys[pos:])
				copy(n.next[pos+1:], n.next[pos:])
				n.keys[pos] = b
				n.next[pos] = nxt
			}
			state = nxt
		}
		li.nodes[state].out = append(li.nodes[state].out, id)
	}
	li.n = len(ids)
	// Fail links in breadth-first order. A node's fail link is the longest
	// proper suffix of its string that is also a prefix of some literal;
	// its outputs are merged so a literal that is a suffix of another is
	// still reported when the longer one is being followed.
	for i, b := range li.nodes[0].keys {
		li.root[b] = li.nodes[0].next[i]
	}
	queue := append([]int32{}, li.nodes[0].next...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		keys, next := li.nodes[cur].keys, li.nodes[cur].next
		for i, b := range keys {
			nxt := next[i]
			target := int32(0)
			for f := li.nodes[cur].fail; ; {
				if f == 0 {
					if t := li.root[b]; t != nxt {
						target = t
					}
					break
				}
				if t := li.step(f, b); t >= 0 {
					target = t
					break
				}
				f = li.nodes[f].fail
			}
			li.nodes[nxt].fail = target
			if out := li.nodes[target].out; len(out) > 0 {
				li.nodes[nxt].out = append(li.nodes[nxt].out, out...)
			}
			queue = append(queue, nxt)
		}
	}
	return li, ids
}

// step returns the transition from state on b, or -1.
func (li *literalIndex) step(state int32, b byte) int32 {
	n := &li.nodes[state]
	keys := n.keys
	// Small sorted slices: a linear scan beats binary search below ~8 keys.
	if len(keys) <= 8 {
		for i, k := range keys {
			if k == b {
				return n.next[i]
			}
		}
		return -1
	}
	lo, hi := 0, len(keys)
	for lo < hi {
		mid := (lo + hi) / 2
		if keys[mid] < b {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(keys) && keys[lo] == b {
		return n.next[lo]
	}
	return -1
}

// present marks hit[id] = true for every literal that occurs in data. hit
// must have length li.n.
func (li *literalIndex) present(data []byte, hit []bool) {
	state := int32(0)
	for _, b := range data {
		for {
			if state == 0 {
				state = li.root[b]
				break
			}
			if t := li.step(state, b); t >= 0 {
				state = t
				break
			}
			state = li.nodes[state].fail
		}
		if out := li.nodes[state].out; len(out) > 0 {
			for _, id := range out {
				hit[id] = true
			}
		}
	}
}

// LiteralCounter exposes the literal index for tooling that measures how
// common literals are across a corpus (tools/litfreq).
type LiteralCounter struct {
	li     *literalIndex
	ids    []int32 // position in the input -> literal id (duplicates share one)
	counts []int
}

// NewLiteralCounter indexes lits in input order.
func NewLiteralCounter(lits []string) *LiteralCounter {
	raw := make([][]byte, len(lits))
	for i, l := range lits {
		raw[i] = []byte(l)
	}
	li, ids := newLiteralIndex(raw)
	c := &LiteralCounter{li: li, ids: make([]int32, len(lits)), counts: make([]int, li.n)}
	for i, l := range lits {
		c.ids[i] = ids[l]
	}
	return c
}

// Len is the number of distinct literals; Present fills hit for one file.
func (c *LiteralCounter) Len() int                        { return c.li.n }
func (c *LiteralCounter) Present(data []byte, hit []bool) { c.li.present(data, hit) }

// Add records one file's hits; Count reports files containing input literal i.
func (c *LiteralCounter) Add(hit []bool) {
	for id, h := range hit {
		if h {
			c.counts[id]++
		}
	}
}
func (c *LiteralCounter) Count(i int) int { return c.counts[c.ids[i]] }

// ID maps an input position to the literal id used in a hit slice.
func (c *LiteralCounter) ID(i int) int32 { return c.ids[i] }
