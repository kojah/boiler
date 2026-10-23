// Package immutable demonstrates Boiler's immutable-type use case: a mutable
// draft graph is published as an independent snapshot and read through
// generated views. generate.boiler gives every field of every struct a rule.
// Host code keeps what needs judgment: the memo table that preserves shared
// and cyclic node identity.
package immutable

// Graph is a mutable draft. Publication detaches every container.
type Graph struct {
	Name   string
	Nodes  []*Node
	Labels map[string]int
	Roots  map[*Node]bool
	// scratch is builder-only working storage; publication zeroes it.
	scratch []int
}

// Node is one graph vertex. Owner may be shared or cyclic.
type Node struct {
	ID    int
	Edges []int
	Owner *Node
}

// Published is an immutable graph publication. Boiler generates its read
// methods; its storage is never exposed.
type Published struct{ graph *Graph }

// publication memoizes copied nodes so shared and cyclic owners keep their
// identity in the snapshot.
type publication struct{ nodes map[*Node]*Node }

// Publish snapshots draft. Later draft changes do not reach the result.
func Publish(draft Graph) Published {
	p := &publication{nodes: map[*Node]*Node{}}
	graph := p.snapshotGraph(draft)
	return Published{graph: &graph}
}

// node copies one node once. The copy is recorded before its fields are
// snapshotted, so a cycle back to n finds it.
func (p *publication) node(n *Node) *Node {
	if n == nil {
		return nil
	}
	if out, ok := p.nodes[n]; ok {
		return out
	}
	out := new(Node)
	p.nodes[n] = out
	*out = p.snapshotNode(*n)
	return out
}
