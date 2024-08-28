package gronos

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hmdsefi/gograph"
)

func TestGronosGraph(t *testing.T) {

	t.Run("Hierarchy of LifecycleFuncs", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		g, errChan := New[string](
			ctx,
			map[string]LifecyleFunc{
				"root": func(ctx context.Context, shutdown <-chan struct{}) error {
					bus, err := UseBusWait(ctx)
					if err != nil {
						return err
					}

					<-bus(func() (<-chan struct{}, Message) {
						return MsgAdd("child1", func(ctx1 context.Context, shutdown <-chan struct{}) error {
							busChild1, err := UseBusWait(ctx1)
							if err != nil {
								return err
							}

							<-busChild1(func() (<-chan struct{}, Message) {
								return MsgAdd("grandchild1", func(ctx11 context.Context, shutdown <-chan struct{}) error {
									<-shutdown
									return nil
								})
							})

							<-shutdown
							return nil
						})
					})

					<-bus(func() (<-chan struct{}, Message) {
						return MsgAdd("child2", func(ctx2 context.Context, shutdown <-chan struct{}) error {
							busChild2, err := UseBusWait(ctx2)
							if err != nil {
								return err
							}

							<-busChild2(func() (<-chan struct{}, Message) {
								return MsgAdd("grandchild2", func(ctx22 context.Context, shutdown <-chan struct{}) error {
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
		time.Sleep(500 * time.Millisecond)

		// Get the graph
		graphChan, msg := MsgRequestGraph[string]()
		g.Send(msg)
		graph := <-graphChan

		// Print the graph structure
		printGraph(t, graph)

		// Verify the graph structure
		vertices := graph.GetAllVertices()
		expectedVertices := 7 // Including the empty string root
		if len(vertices) != expectedVertices {
			t.Errorf("Expected %d vertices in the graph, got %d", expectedVertices, len(vertices))
		}

		// Helper function to check if a vertex has an edge to another vertex
		hasEdgeTo := func(from, to *gograph.Vertex[string]) bool {
			return len(graph.GetAllEdges(from, to)) > 0
		}

		// Check empty string root connections
		emptyRootVertex := graph.GetVertexByID(g.computedRootKey)
		if emptyRootVertex == nil {
			t.Error("Empty string root vertex should exist")
		} else {
			if !hasEdgeTo(emptyRootVertex, graph.GetVertexByID("root")) {
				t.Error("Empty string root should have an edge to 'root'")
			}
			if !hasEdgeTo(emptyRootVertex, graph.GetVertexByID("standalone")) {
				t.Error("Empty string root should have an edge to 'standalone'")
			}
		}

		// Check "root" connections
		rootVertex := graph.GetVertexByID("root")
		if rootVertex == nil {
			t.Error("Root vertex should exist")
		} else {
			if !hasEdgeTo(rootVertex, graph.GetVertexByID("child1")) && !hasEdgeTo(rootVertex, graph.GetVertexByID("child2")) {
				t.Error("Root should have an edge to either 'child1' or 'child2'")
			}
		}

		// Check child1 and its grandchild
		child1Vertex := graph.GetVertexByID("child1")
		if child1Vertex == nil {
			t.Error("child1 vertex should exist")
		} else {
			if !hasEdgeTo(child1Vertex, graph.GetVertexByID("grandchild1")) {
				t.Error("child1 should have an edge to 'grandchild1'")
			}
		}

		// Check child2 and its grandchild
		child2Vertex := graph.GetVertexByID("child2")
		if child2Vertex == nil {
			t.Error("child2 vertex should exist")
		} else {
			if !hasEdgeTo(child2Vertex, graph.GetVertexByID("grandchild2")) {
				t.Error("child2 should have an edge to 'grandchild2'")
			}
		}

		// Check standalone
		standaloneVertex := graph.GetVertexByID("standalone")
		if standaloneVertex == nil {
			t.Error("standalone vertex should exist")
		} else {
			for _, v := range vertices {
				if v != standaloneVertex && hasEdgeTo(standaloneVertex, v) {
					t.Errorf("standalone should have no children, but has edge to %s", v.Label())
				}
			}
		}

		// Shutdown and wait
		g.Shutdown()
		g.Wait()
	})
}

func printGraph(t *testing.T, graph gograph.Graph[string]) {
	t.Log("Graph structure:")
	vertices := graph.GetAllVertices()
	for _, v := range vertices {
		var edges []string
		for _, to := range vertices {
			if len(graph.GetAllEdges(v, to)) > 0 {
				edges = append(edges, to.Label())
			}
		}
		t.Logf("Vertex %s -> %s", v.Label(), strings.Join(edges, ", "))
	}
}

// func verifyGraphStructure(t *testing.T, graph gograph.Graph[string], computedRootKey string) {
// 	// Check essential relationships
// 	essentialRelationships := []struct {
// 		from, to string
// 	}{
// 		{computedRootKey, "root"},
// 		{computedRootKey, "standalone"},
// 		{"root", "child1"},
// 		{"root", "child2"},
// 		{"child1", "grandchild1"},
// 		{"child2", "grandchild2"},
// 	}

// 	vertices := graph.GetAllVertices()

// 	for _, rel := range essentialRelationships {

// 		isEdge, err := graph.IsEdge(rel.from, rel.to)
// 		if err != nil {
// 			t.Errorf("Error checking edge from %s to %s: %v", rel.from, rel.to, err)
// 		} else if !isEdge {
// 			t.Errorf("Expected direct edge from %s to %s, but it doesn't exist", rel.from, rel.to)
// 		}
// 	}

// 	// Check for unexpected direct relationships
// 	unexpectedDirectRelationships := []struct {
// 		from, to string
// 	}{
// 		{"root", "grandchild1"},
// 		{"root", "grandchild2"},
// 		{"child1", "grandchild2"},
// 		{"child2", "grandchild1"},
// 		{"standalone", "child1"},
// 		{"standalone", "child2"},
// 		{"standalone", "grandchild1"},
// 		{"standalone", "grandchild2"},
// 	}

// 	for _, rel := range unexpectedDirectRelationships {
// 		isEdge, err := graph.IsEdge(rel.from, rel.to)
// 		if err != nil {
// 			t.Errorf("Error checking edge from %s to %s: %v", rel.from, rel.to, err)
// 		} else if isEdge {
// 			t.Errorf("Unexpected direct edge from %s to %s", rel.from, rel.to)
// 		}
// 	}
// }

// // func printGraph(t *testing.T, graph *dag.DAG, rootKey string) {
// // 	t.Log("Graph hierarchy:")
// // 	printNodeDetailed(t, graph, rootKey, 0)
// // }

// // func printNodeDetailed(t *testing.T, graph *dag.DAG, nodeID string, depth int) {
// // 	indent := strings.Repeat("  ", depth)
// // 	// t.Logf("%s%s", indent, nodeID)

// // 	children := []string{"standalone", "root", "child1", "child2", "grandchild1", "grandchild2"}
// // 	for _, childID := range children {
// // 		isDirectEdge, err := graph.IsEdge(nodeID, childID)
// // 		if err != nil {
// // 			// t.Errorf("Error checking edge between %s and %s: %v", nodeID, childID, err)
// // 			continue
// // 		}

// // 		if isDirectEdge {
// // 			t.Logf("%s  -> %s", indent, childID)
// // 			newdept := depth + 1
// // 			printNodeDetailed(t, graph, childID, newdept)
// // 		}
// // 	}
// // }
