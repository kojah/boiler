package immutable

import "testing"

// TestPublishDetachesDraft checks the immutable-publication contract of the
// generated snapshot: later draft writes never reach the publication, and
// shared or cyclic nodes keep one identity.
func TestPublishDetachesDraft(t *testing.T) {
	owner := &Node{ID: 1}
	owner.Owner = owner
	draft := Graph{
		Name:   "g",
		Nodes:  []*Node{owner, {ID: 2, Edges: []int{1}, Owner: owner}},
		Labels: map[string]int{"a": 1},
		Roots:  map[*Node]bool{owner: true},
	}
	published := Publish(draft)

	draft.Nodes[1].Edges[0] = 99
	draft.Nodes[0] = &Node{ID: 7}
	draft.Labels["a"] = 2
	owner.ID = 5

	if published.Name() != "g" || published.NodeCount() != 2 {
		t.Fatalf("published %q with %d nodes", published.Name(), published.NodeCount())
	}
	first, second := published.Node(0), published.Node(1)
	if first.ID() != 1 || second.Edges().At(0) != 1 {
		t.Fatalf("draft writes reached the publication: id %d, edge %d", first.ID(), second.Edges().At(0))
	}
	if label, _ := published.Labels().Get("a"); label != 1 {
		t.Fatalf("draft label write reached the publication: %d", label)
	}
	if second.Owner() != first || first.Owner() != first {
		t.Fatalf("shared and cyclic owners lost their identity")
	}
	visited := 0
	for range published.Nodes() {
		visited++
	}
	if visited != 2 {
		t.Fatalf("visited %d nodes", visited)
	}
}

// TestEmptyViews checks that views of nothing read as zero values.
func TestEmptyViews(t *testing.T) {
	var empty Published
	if empty.Name() != "" || empty.NodeCount() != 0 || empty.Labels().Len() != 0 {
		t.Fatalf("empty publication is not zero")
	}
	var node NodeView
	if node.Valid() || node.ID() != 0 || node.Edges().Len() != 0 || node.Owner().Valid() {
		t.Fatalf("empty node view is not zero")
	}
}
