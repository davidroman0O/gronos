package gronos

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/heimdalr/dag"
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
						return NewMessageAddLifecycleFunction("child1", func(ctx1 context.Context, shutdown <-chan struct{}) error {
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

		{
			// that's the new direction of the next iteration of the message system
			msg := NewMessageAddLifecycleFunction(
				"test",
				func(ctx1 context.Context, shutdown <-chan struct{}) error {
					return nil
				})
			// That's exact what i wanted
			switch value := g.enqueue(nil, msg).(type) {
			case *Success[MessageAddLifecycleFunction[string]]:
				fmt.Println(value)
			case *Failure:
				fmt.Println(value.Err)
			}
		}

		// Wait for all LifecycleFuncs to start
		time.Sleep(500 * time.Millisecond)

		// Get the graph
		graphChan, msg := MsgRequestGraph[string]()
		g.Send(msg)
		graph := <-graphChan

		fmt.Print(graph.String())
		// Print and verify the graph structure
		// printGraph(t, graph, g.computedRootKey)
		verifyGraphStructure(t, graph, g.computedRootKey)

		// Shutdown and wait
		g.Shutdown()
		g.Wait()
	})
}

func verifyGraphStructure(t *testing.T, graph *dag.DAG, computedRootKey string) {
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
		isEdge, err := graph.IsEdge(rel.from, rel.to)
		if err != nil {
			t.Errorf("Error checking edge from %s to %s: %v", rel.from, rel.to, err)
		} else if !isEdge {
			t.Errorf("Expected direct edge from %s to %s, but it doesn't exist", rel.from, rel.to)
		}
	}

	// Check for unexpected direct relationships
	unexpectedDirectRelationships := []struct {
		from, to string
	}{
		{"root", "grandchild1"},
		{"root", "grandchild2"},
		{"child1", "grandchild2"},
		{"child2", "grandchild1"},
		{"standalone", "child1"},
		{"standalone", "child2"},
		{"standalone", "grandchild1"},
		{"standalone", "grandchild2"},
	}

	for _, rel := range unexpectedDirectRelationships {
		isEdge, err := graph.IsEdge(rel.from, rel.to)
		if err != nil {
			t.Errorf("Error checking edge from %s to %s: %v", rel.from, rel.to, err)
		} else if isEdge {
			t.Errorf("Unexpected direct edge from %s to %s", rel.from, rel.to)
		}
	}
}

// func printGraph(t *testing.T, graph *dag.DAG, rootKey string) {
// 	t.Log("Graph hierarchy:")
// 	printNodeDetailed(t, graph, rootKey, 0)
// }

// func printNodeDetailed(t *testing.T, graph *dag.DAG, nodeID string, depth int) {
// 	indent := strings.Repeat("  ", depth)
// 	// t.Logf("%s%s", indent, nodeID)

// 	children := []string{"standalone", "root", "child1", "child2", "grandchild1", "grandchild2"}
// 	for _, childID := range children {
// 		isDirectEdge, err := graph.IsEdge(nodeID, childID)
// 		if err != nil {
// 			// t.Errorf("Error checking edge between %s and %s: %v", nodeID, childID, err)
// 			continue
// 		}

// 		if isDirectEdge {
// 			t.Logf("%s  -> %s", indent, childID)
// 			newdept := depth + 1
// 			printNodeDetailed(t, graph, childID, newdept)
// 		}
// 	}
// }
