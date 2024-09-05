package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/davidroman0O/gronos/etcd"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	fmt.Println("--- Starting Comprehensive etcd Demonstration ---")

	demonstrateLeaderEmbedWithFollowers(ctx)
	demonstrateLeaderRemoteWithFollowers(ctx)
	demonstrateStandalone(ctx)

	fmt.Println("--- Comprehensive etcd Demonstration Completed ---")
}

func demonstrateLeaderEmbedWithFollowers(ctx context.Context) {
	fmt.Println("\n=== Demonstrating Leader Embed with Followers ===")

	leader, err := etcd.NewEtcd(
		etcd.WithMode(etcd.ModeLeaderEmbed),
		etcd.WithDataDir("./data/leader_embed"),
		etcd.WithPorts(2380, 2379),
	)
	if err != nil {
		log.Fatalf("Failed to create leader: %v", err)
	}
	defer leader.Close()

	followers := make([]*etcd.EtcdManager, 2)
	for i := range followers {
		follower, err := etcd.NewEtcd(
			etcd.WithMode(etcd.ModeFollower),
			etcd.WithEndpoints([]string{"localhost:2379"}),
		)
		if err != nil {
			log.Fatalf("Failed to create follower %d: %v", i, err)
		}
		defer follower.Close()
		followers[i] = follower
	}

	demonstrateAllOperations(ctx, leader, followers, "leader_embed")
}

func demonstrateLeaderRemoteWithFollowers(ctx context.Context) {
	fmt.Println("\n=== Demonstrating Leader Remote with Followers ===")

	etcdServer, err := etcd.NewEtcd(
		etcd.WithMode(etcd.ModeStandalone),
		etcd.WithDataDir("./data/remote_etcd"),
		etcd.WithPorts(2382, 2381),
	)
	if err != nil {
		log.Fatalf("Failed to create etcd server: %v", err)
	}
	defer etcdServer.Close()

	leader, err := etcd.NewEtcd(
		etcd.WithMode(etcd.ModeLeaderRemote),
		etcd.WithEndpoints([]string{"localhost:2381"}),
	)
	if err != nil {
		log.Fatalf("Failed to create remote leader: %v", err)
	}
	defer leader.Close()

	followers := make([]*etcd.EtcdManager, 2)
	for i := range followers {
		follower, err := etcd.NewEtcd(
			etcd.WithMode(etcd.ModeFollower),
			etcd.WithEndpoints([]string{"localhost:2381"}),
		)
		if err != nil {
			log.Fatalf("Failed to create follower %d: %v", i, err)
		}
		defer follower.Close()
		followers[i] = follower
	}

	demonstrateAllOperations(ctx, leader, followers, "leader_remote")
}

func demonstrateStandalone(ctx context.Context) {
	fmt.Println("\n=== Demonstrating Standalone ===")

	standalone, err := etcd.NewEtcd(
		etcd.WithMode(etcd.ModeStandalone),
		etcd.WithDataDir("./data/standalone"),
		etcd.WithPorts(2384, 2383),
	)
	if err != nil {
		log.Fatalf("Failed to create standalone etcd: %v", err)
	}
	defer standalone.Close()

	demonstrateAllOperations(ctx, standalone, nil, "standalone")
}

func demonstrateAllOperations(ctx context.Context, leader *etcd.EtcdManager, followers []*etcd.EtcdManager, prefix string) {
	demonstrateCRUD(ctx, leader, followers, prefix)
	demonstrateTransactions(ctx, leader, prefix)
	demonstrateWatchers(ctx, leader, followers, prefix)
	demonstrateCommands(ctx, leader, followers, prefix)
}

func demonstrateCRUD(ctx context.Context, leader *etcd.EtcdManager, followers []*etcd.EtcdManager, prefix string) {
	fmt.Println("--- Demonstrating CRUD Operations ---")

	key := fmt.Sprintf("%s_key", prefix)
	value := fmt.Sprintf("%s_value", prefix)

	// Put
	err := leader.Put(ctx, key, value)
	if err != nil {
		log.Printf("Leader failed to put value: %v", err)
	} else {
		fmt.Printf("Leader put value: %s = %s\n", key, value)
	}

	// Get
	readValue, err := leader.Get(ctx, key)
	if err != nil {
		log.Printf("Leader failed to get value: %v", err)
	} else {
		fmt.Printf("Leader read value: %s = %s\n", key, readValue)
	}

	// Followers read
	for i, follower := range followers {
		readValue, err := follower.Get(ctx, key)
		if err != nil {
			log.Printf("Follower %d failed to get value: %v", i, err)
		} else {
			fmt.Printf("Follower %d read value: %s = %s\n", i, key, readValue)
		}
	}

	// Update
	newValue := fmt.Sprintf("%s_updated", value)
	err = leader.Put(ctx, key, newValue)
	if err != nil {
		log.Printf("Leader failed to update value: %v", err)
	} else {
		fmt.Printf("Leader updated value: %s = %s\n", key, newValue)
	}

	// Delete
	err = leader.Delete(ctx, key)
	if err != nil {
		log.Printf("Leader failed to delete value: %v", err)
	} else {
		fmt.Printf("Leader deleted key: %s\n", key)
	}
}

func demonstrateTransactions(ctx context.Context, leader *etcd.EtcdManager, prefix string) {
	fmt.Println("--- Demonstrating Transactions ---")

	key1 := fmt.Sprintf("%s_tx_key1", prefix)
	key2 := fmt.Sprintf("%s_tx_key2", prefix)
	value1 := fmt.Sprintf("%s_tx_value1", prefix)
	value2 := fmt.Sprintf("%s_tx_value2", prefix)

	txn, err := leader.BeginTxn(ctx, fmt.Sprintf("%s_lock", prefix))
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}

	txn.Put(key1, value1)
	txn.Put(key2, value2)

	resp, err := txn.Commit()
	if err != nil {
		log.Printf("Transaction failed: %v", err)
	} else {
		fmt.Printf("Transaction succeeded: %v\n", resp.Succeeded)
		fmt.Printf("Put %s = %s and %s = %s in a single transaction\n", key1, value1, key2, value2)
	}

	// Verify the transaction results
	val1, _ := leader.Get(ctx, key1)
	val2, _ := leader.Get(ctx, key2)
	fmt.Printf("After transaction: %s = %s, %s = %s\n", key1, val1, key2, val2)
}

func demonstrateWatchers(ctx context.Context, leader *etcd.EtcdManager, followers []*etcd.EtcdManager, prefix string) {
	fmt.Println("--- Demonstrating Watchers ---")

	watchKey := fmt.Sprintf("%s_watch_key", prefix)

	var wg sync.WaitGroup

	// Create a new context with a timeout for the watcher
	watchCtx, cancelWatch := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWatch()

	// Start a watcher on a follower
	if len(followers) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			watchChan := followers[0].Watch(watchCtx, watchKey)
			for watchResp := range watchChan {
				for _, ev := range watchResp.Events {
					fmt.Printf("Watcher received event: %s %q : %q\n", ev.Type, ev.Kv.Key, ev.Kv.Value)
				}
			}
			fmt.Println("Watch channel closed")
		}()
	}

	// Perform some operations on the watched key
	time.Sleep(time.Second) // Give some time for the watch to start

	leader.Put(ctx, watchKey, "initial_value")
	time.Sleep(time.Second)
	leader.Put(ctx, watchKey, "updated_value")
	time.Sleep(time.Second)
	leader.Delete(ctx, watchKey)

	// Wait for the watcher to complete
	wg.Wait()
	fmt.Println("Watcher demonstration completed")
}

func demonstrateCommands(ctx context.Context, leader *etcd.EtcdManager, followers []*etcd.EtcdManager, prefix string) {
	fmt.Println("--- Demonstrating Commands ---")

	var wg sync.WaitGroup

	// Create a new context with a timeout for the command watcher
	cmdCtx, cancelCmd := context.WithTimeout(ctx, 10*time.Second)
	defer cancelCmd()

	// Start a command watcher on a follower
	if len(followers) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			watchChan := followers[0].WatchCommands(cmdCtx)
			for watchResp := range watchChan {
				for _, ev := range watchResp.Events {
					var cmd etcd.Command
					if err := json.Unmarshal(ev.Kv.Value, &cmd); err != nil {
						log.Printf("Failed to unmarshal command: %v", err)
						continue
					}
					fmt.Printf("Received command: %+v\n", cmd)

					// Simulate command processing
					time.Sleep(time.Second)
					response := etcd.Response{
						CommandID: cmd.ID,
						Success:   true,
						Message:   "Command processed successfully",
					}

					if err := followers[0].SendResponse(response); err != nil {
						log.Printf("Failed to send response: %v", err)
					} else {
						fmt.Println("Response sent successfully")
					}
				}
			}
			fmt.Println("Command watch channel closed")
		}()
	}

	// Send a command
	cmd := etcd.Command{
		ID:     fmt.Sprintf("%s_cmd1", prefix),
		Type:   "test",
		NodeID: "node1",
		Data:   map[string]string{"key": "value"},
	}

	respChan, err := leader.SendCommand(cmd)
	if err != nil {
		log.Printf("Failed to send command: %v", err)
	} else {
		select {
		case resp := <-respChan:
			fmt.Printf("Received response: %+v\n", resp)
		case <-time.After(5 * time.Second):
			fmt.Println("Timeout waiting for response")
		}
	}

	// Wait for the command watcher to complete
	wg.Wait()
	fmt.Println("Command demonstration completed")
}
