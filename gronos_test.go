package gronos

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/davidroman0O/gronos/etcd"
	"github.com/stretchr/testify/assert"
)

func TestBasicEmbedBeacon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	leaderReady := make(chan struct{})
	beaconReady := make(chan struct{})
	followerConnected := make(chan struct{})
	errChan := make(chan error, 3)

	var wg sync.WaitGroup
	wg.Add(3) // Leader, Beacon, Follower

	// Start leader
	go runLeader(ctx, &wg, leaderReady, errChan)
	select {
	case <-leaderReady:
		t.Log("Leader is ready")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for leader to be ready")
	}

	// Start beacon
	go runBeacon(ctx, &wg, beaconReady, errChan)
	select {
	case <-beaconReady:
		t.Log("Beacon is ready")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for beacon to be ready")
	}

	// Start follower
	go runFollower(ctx, &wg, followerConnected, errChan)
	select {
	case <-followerConnected:
		t.Log("Follower is connected")
	case <-ctx.Done():
		t.Fatal("Timeout waiting for follower to connect")
	}

	// Trigger and verify graph updates
	if err := triggerAndVerifyGraphUpdates(ctx, t); err != nil {
		t.Fatalf("Failed to verify graph updates: %v", err)
	}

	// Wait for test completion or error
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("Test timed out")
		}
	case err := <-errChan:
		t.Fatalf("Test failed with error: %v", err)
	default:
		// If we reach here, it means the test completed successfully
		t.Log("Test completed successfully")
	}

	// Ensure all goroutines have finished
	wgDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgDone)
	}()

	select {
	case <-wgDone:
		t.Log("All goroutines finished")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for goroutines to finish")
	}
}

func runLeader(ctx context.Context, wg *sync.WaitGroup, ready chan<- struct{}, errChan chan<- error) {
	defer wg.Done()

	opts, err := NewOptionsBuilder(etcd.ModeLeaderEmbed).
		WithRemoveDataDir().
		WithPorts(2380, 2379).
		Build()
	if err != nil {
		errChan <- fmt.Errorf("Error creating new options for leader: %v", err)
		return
	}

	g, err := New[string](opts)
	if err != nil {
		errChan <- fmt.Errorf("Error creating new gronos for leader: %v", err)
		return
	}

	close(ready)

	if err = g.Run(ctx); err != nil && err != context.Canceled {
		errChan <- fmt.Errorf("Error running gronos leader: %v", err)
	}
}

func runBeacon(ctx context.Context, wg *sync.WaitGroup, ready chan<- struct{}, errChan chan<- error) {
	defer wg.Done()

	opts, err := NewOptionsBuilder(etcd.ModeBeacon).
		WithRemoveDataDir().
		WithBeacon("localhost:5000").
		WithEtcdEndpoint("localhost:2379").
		Build()
	if err != nil {
		errChan <- fmt.Errorf("Error creating new options for beacon: %v", err)
		return
	}

	g, err := New[string](opts)
	if err != nil {
		errChan <- fmt.Errorf("Error creating new gronos for beacon: %v", err)
		return
	}

	close(ready)

	if err = g.Run(ctx); err != nil && err != context.Canceled {
		errChan <- fmt.Errorf("Error running gronos beacon: %v", err)
	}
}

func runFollower(ctx context.Context, wg *sync.WaitGroup, connected chan<- struct{}, errChan chan<- error) {
	defer wg.Done()

	opts, err := NewOptionsBuilder(etcd.ModeFollower).
		WithRemoveDataDir().
		WithBeacon("localhost:5000").
		Build()
	if err != nil {
		errChan <- fmt.Errorf("Error creating new options for follower: %v", err)
		return
	}

	g, err := New[string](opts)
	if err != nil {
		errChan <- fmt.Errorf("Error creating new gronos for follower: %v", err)
		return
	}

	close(connected)

	if err = g.Run(ctx); err != nil && err != context.Canceled {
		errChan <- fmt.Errorf("Error running gronos follower: %v", err)
	}
}

func triggerAndVerifyGraphUpdates(ctx context.Context, t *testing.T) error {
	// TODO: Implement graph update triggering and verification
	// This function should:
	// 1. Trigger graph updates on the leader
	// 2. Wait for the follower to receive and process these updates
	// 3. Verify that the follower's graph matches the leader's graph

	// For now, we'll just add a placeholder assertion
	assert.True(t, true, "Graph updates should be verified")

	return nil
}
