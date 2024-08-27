package gronos

import (
	"context"
	"testing"
	"time"

	"github.com/heimdalr/dag"
)

func TestGronosGraph(t *testing.T) {
	t.Run("Hierarchy of LifecycleFuncs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		g, errChan := New[string](ctx, map[string]LifecyleFunc{
			"root": func(ctx context.Context, shutdown <-chan struct{}) error {
				bus, err := UseBusWait(ctx)
				if err != nil {
					return err
				}

				<-bus(func() (<-chan struct{}, Message) {
					return MsgAdd("child1", func(ctx context.Context, shutdown <-chan struct{}) error {
						bus, err := UseBusWait(ctx)
						if err != nil {
							return err
						}

						<-bus(func() (<-chan struct{}, Message) {
							return MsgAdd("grandchild1", func(ctx context.Context, shutdown <-chan struct{}) error {
								<-shutdown
								return nil
							})
						})

						<-shutdown
						return nil
					})
				})

				<-bus(func() (<-chan struct{}, Message) {
					return MsgAdd("child2", func(ctx context.Context, shutdown <-chan struct{}) error {
						bus, err := UseBusWait(ctx)
						if err != nil {
							return err
						}

						<-bus(func() (<-chan struct{}, Message) {
							return MsgAdd("grandchild2", func(ctx context.Context, shutdown <-chan struct{}) error {
								<-shutdown
								return nil
							})
						})

						<-shutdown
						return nil
					})
				})

				<-shutdown
				return nil
			},
			"standalone": func(ctx context.Context, shutdown <-chan struct{}) error {
				<-shutdown
				return nil
			},
		})

		// Handle errors
		go func() {
			for err := range errChan {
				t.Logf("Error received: %v", err)
			}
		}()

		// Wait for all LifecycleFuncs to start
		time.Sleep(100 * time.Millisecond)

		// Get the graph
		graphChan, msg := MsgRequestGraph[string]()
		g.Send(msg)
		graph := <-graphChan

		// Print and verify the graph structure
		verifyGraph(t, graph, g.computedRootKey)

		// Shutdown and wait
		g.Shutdown()
		g.Wait()
	})
}

func verifyGraph(t *testing.T, graph *dag.DAG, computedRootKey string) {
	printGraph(t, graph)

	nodes := graph.GetVertices()
	expectedNodes := 7 // Including the empty string root
	if len(nodes) != expectedNodes {
		t.Errorf("Expected %d nodes in the graph, got %d", expectedNodes, len(nodes))
	}

	// Helper function to check if a node has an edge to another node
	hasEdgeTo := func(from, to string) bool {
		ok, err := graph.IsEdge(from, to)
		if err != nil {
			t.Errorf("Error checking edge from %s to %s: %v", from, to, err)
			return false
		}
		return ok
	}

	// Check essential relationships
	essentialRelationships := []struct {
		from, to string
	}{
		{computedRootKey, "root"},
		{computedRootKey, "standalone"},
		{"root", "child1"},
		{"root", "child2"},
		{"child1", "grandchild1"},
		{"child2", "grandchild2"},
	}

	for _, rel := range essentialRelationships {
		if !hasEdgeTo(rel.from, rel.to) {
			t.Errorf("Expected edge from %s to %s, but it doesn't exist", rel.from, rel.to)
		}
	}

	// Check for unexpected relationships
	unexpectedRelationships := []struct {
		from, to string
	}{
		{"standalone", "root"},
		{"standalone", "child1"},
		{"standalone", "child2"},
		{"standalone", "grandchild1"},
		{"standalone", "grandchild2"},
		{"grandchild1", "child1"},
		{"grandchild1", "child2"},
		{"grandchild1", "grandchild2"},
		{"grandchild2", "child1"},
		{"grandchild2", "child2"},
		{"grandchild2", "grandchild1"},
	}

	for _, rel := range unexpectedRelationships {
		if hasEdgeTo(rel.from, rel.to) {
			t.Errorf("Unexpected edge from %s to %s", rel.from, rel.to)
		}
	}
}

type testVisitor struct {
	cb func(v dag.Vertexer)
}

func (pv testVisitor) Visit(v dag.Vertexer) {
	pv.cb(v)
}

func printGraph(t *testing.T, graph *dag.DAG) {
	t.Log("Graph structure:")
	// nodes := graph.GetVertices()
	visitor := &testVisitor{
		cb: func(v dag.Vertexer) {
			id, value := v.Vertex()
			t.Logf("Node %s - %v", id, value)
		},
	}
	graph.OrderedWalk(visitor)
	// fmt.Println("Values:", visitor.Values)
	// for _, v := range visitor.Values {
	// 	t.Log(v)
	// }
	// graph.GetChildren()
	// for _, v := range nodes {
	// 	var edgeIDs []string
	// 	for _, e := range edges {
	// 		if e.From() == v {
	// 			edgeIDs = append(edgeIDs, e.To().ID())
	// 		}
	// 	}
	// 	t.Logf("Node %s -> %s", v.ID(), strings.Join(edgeIDs, ", "))
	// }
}
