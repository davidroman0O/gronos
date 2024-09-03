package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const testDataDir = "./test_data"

func cleanupDataDir(t *testing.T) {
	t.Helper()
	err := os.RemoveAll(testDataDir)
	require.NoError(t, err, "Failed to clean up test data directory")
}

type testEtcdManager struct {
	t  *testing.T
	em *EtcdManager
}

func newTestEtcdManager(t *testing.T, mode Mode, opts ...Option) *testEtcdManager {
	t.Helper()
	cleanupDataDir(t)

	dataDir := filepath.Join(testDataDir, fmt.Sprintf("%s_%d", mode.String(), time.Now().UnixNano()))
	opts = append(opts,
		WithMode(mode),
		WithDataDir(dataDir),
		WithDialTimeout(2*time.Second),
		WithRequestTimeout(2*time.Second),
		WithRetryAttempts(3),
		WithRetryDelay(100*time.Millisecond),
	)

	em, err := NewEtcdManager(opts...)
	require.NoError(t, err, "Failed to create EtcdManager")

	t.Cleanup(func() {
		em.Close()
		cleanupDataDir(t)
	})

	return &testEtcdManager{t: t, em: em}
}

func TestEtcdManagerInitialization(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithPorts(2380, 2379)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{"localhost:2379"})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{"localhost:2379"})}},
		{"Standalone", ModeStandalone, []Option{WithPorts(2382, 2381)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupDataDir(t)
			defer cleanupDataDir(t)

			em := newTestEtcdManager(t, tt.mode, tt.opts...)
			assert.NotNil(t, em.em, "EtcdManager should be created successfully")
		})
	}
}

func TestEtcdManagerCRUD(t *testing.T) {
	// Start a standalone etcd server for LeaderRemote and Follower tests
	standaloneServer, err := NewEtcdManager(
		WithMode(ModeStandalone),
		WithDataDir(t.TempDir()),
		WithPorts(2380, 2379),
	)
	require.NoError(t, err, "Failed to create standalone etcd server")
	defer standaloneServer.Close()

	// Wait for the server to be ready
	time.Sleep(1 * time.Second)

	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithDataDir(t.TempDir()), WithPorts(2382, 2381)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{"localhost:2379"})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{"localhost:2379"})}},
		{"Standalone", ModeStandalone, []Option{WithDataDir(t.TempDir()), WithPorts(2384, 2383)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em, err := NewEtcdManager(append(tt.opts, WithMode(tt.mode))...)
			require.NoError(t, err, "Failed to create EtcdManager")
			defer em.Close()

			ctx := context.Background()

			t.Run("Put and Get", func(t *testing.T) {
				key := "test_key"
				value := "test_value"

				err := em.Put(ctx, key, value)
				assert.NoError(t, err, "Put operation failed")

				got, err := em.Get(ctx, key)
				assert.NoError(t, err, "Get operation failed")
				assert.Equal(t, value, got, "Retrieved value doesn't match the put value")
			})

			t.Run("Delete", func(t *testing.T) {
				key := "delete_key"
				value := "delete_value"

				err := em.Put(ctx, key, value)
				assert.NoError(t, err, "Put operation failed")

				err = em.Delete(ctx, key)
				assert.NoError(t, err, "Delete operation failed")

				_, err = em.Get(ctx, key)
				assert.Error(t, err, "Get operation should fail for a deleted key")
				assert.Contains(t, err.Error(), "key not found", "Error should indicate that the key was not found")
			})
		})
	}
}

func TestEtcdManagerWatch(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	// Start a standalone etcd server for LeaderRemote and Follower tests
	standaloneServer, err := NewEtcdManager(
		WithMode(ModeStandalone),
		WithDataDir(t.TempDir()),
		WithPorts(2380, 2379),
	)
	require.NoError(t, err, "Failed to create standalone etcd server")
	defer standaloneServer.Close()

	// Wait for the server to be ready
	time.Sleep(1 * time.Second)

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			var em *EtcdManager
			var err error

			switch mode {
			case ModeLeaderEmbed:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2382, 2381),
				)
			case ModeLeaderRemote, ModeFollower:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithEndpoints([]string{"localhost:2379"}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err, "Failed to create EtcdManager")
			defer em.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			watchKey := "watch_key"
			watchChan := em.Watch(ctx, watchKey)

			go func() {
				time.Sleep(100 * time.Millisecond)
				em.Put(ctx, watchKey, "value1")
				time.Sleep(100 * time.Millisecond)
				em.Put(ctx, watchKey, "value2")
				time.Sleep(100 * time.Millisecond)
				em.Delete(ctx, watchKey)
			}()

			expectedEvents := []struct {
				eventType string
				value     string
			}{
				{"PUT", "value1"},
				{"PUT", "value2"},
				{"DELETE", ""},
			}

			for _, expected := range expectedEvents {
				select {
				case watchResp := <-watchChan:
					assert.Len(t, watchResp.Events, 1, "Expected one event")
					event := watchResp.Events[0]
					assert.Equal(t, expected.eventType, event.Type.String(), "Unexpected event type")
					assert.Equal(t, watchKey, string(event.Kv.Key), "Unexpected key")
					if expected.eventType != "DELETE" {
						assert.Equal(t, expected.value, string(event.Kv.Value), "Unexpected value")
					}
				case <-ctx.Done():
					t.Fatal("Watch timed out")
				}
			}
		})
	}
}

func TestEtcdManagerTransaction(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			t.Run("Successful Transaction", func(t *testing.T) {
				txn, err := em.em.BeginTxn(ctx, "test_lock")
				require.NoError(t, err)

				txn.Put("key1", "value1")
				txn.Put("key2", "value2")

				resp, err := txn.Commit()
				assert.NoError(t, err)
				assert.True(t, resp.Succeeded)

				value1, err := em.em.Get(ctx, "key1")
				assert.NoError(t, err)
				assert.Equal(t, "value1", value1)

				value2, err := em.em.Get(ctx, "key2")
				assert.NoError(t, err)
				assert.Equal(t, "value2", value2)
			})

			t.Run("Transaction Rollback", func(t *testing.T) {
				txn, err := em.em.BeginTxn(ctx, "test_lock")
				require.NoError(t, err)

				txn.Put("key3", "value3")
				txn.Put("key4", "value4")

				err = txn.Rollback()
				assert.NoError(t, err)

				_, err = em.em.Get(ctx, "key3")
				assert.Error(t, err, "Key should not exist after rollback")

				_, err = em.em.Get(ctx, "key4")
				assert.Error(t, err, "Key should not exist after rollback")
			})
		})
	}
}

func TestEtcdManagerCommandResponse(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			watchChan := em.em.WatchCommands(ctx)

			cmd := Command{
				ID:     "test_cmd",
				Type:   "test",
				NodeID: "node1",
				Data:   map[string]string{"key": "value"},
			}

			respChan, err := em.em.SendCommand(cmd)
			require.NoError(t, err)

			go func() {
				select {
				case watchResp := <-watchChan:
					for _, ev := range watchResp.Events {
						var receivedCmd Command
						json.Unmarshal(ev.Kv.Value, &receivedCmd)
						assert.Equal(t, cmd, receivedCmd)

						response := Response{
							CommandID: receivedCmd.ID,
							Success:   true,
							Message:   "Command processed successfully",
						}
						em.em.SendResponse(response)
					}
				case <-ctx.Done():
					t.Error("Watch timed out")
				}
			}()

			select {
			case resp := <-respChan:
				assert.Equal(t, cmd.ID, resp.CommandID)
				assert.True(t, resp.Success)
				assert.Equal(t, "Command processed successfully", resp.Message)
			case <-ctx.Done():
				t.Fatal("Timeout waiting for response")
			}
		})
	}
}

func TestEtcdManagerMultipleNodes(t *testing.T) {
	cleanupDataDir(t)
	defer cleanupDataDir(t)

	leader := newTestEtcdManager(t, ModeLeaderEmbed, WithPorts(2392, 2391))
	follower1 := newTestEtcdManager(t, ModeFollower, WithEndpoints([]string{"localhost:2391"}))
	follower2 := newTestEtcdManager(t, ModeFollower, WithEndpoints([]string{"localhost:2391"}))

	ctx := context.Background()

	key := "multi_node_key"
	value := "multi_node_value"

	err := leader.em.Put(ctx, key, value)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	value1, err := follower1.em.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, value, value1)

	value2, err := follower2.em.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, value, value2)
}

func TestEtcdManagerConcurrency(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			const numGoroutines = 10
			const numOperations = 100

			var wg sync.WaitGroup
			wg.Add(numGoroutines)

			for i := 0; i < numGoroutines; i++ {
				go func(id int) {
					defer wg.Done()
					for j := 0; j < numOperations; j++ {
						key := fmt.Sprintf("concurrent_key_%d_%d", id, j)
						value := fmt.Sprintf("value_%d_%d", id, j)

						err := em.em.Put(ctx, key, value)
						assert.NoError(t, err)

						got, err := em.em.Get(ctx, key)
						assert.NoError(t, err)
						assert.Equal(t, value, got)

						err = em.em.Delete(ctx, key)
						assert.NoError(t, err)
					}
				}(i)
			}

			wg.Wait()
		})
	}
}

func TestEtcdManagerLargeValues(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			largeValue := make([]byte, 1024*1024) // 1MB
			for i := range largeValue {
				largeValue[i] = byte(i % 256)
			}

			key := "large_value_key"
			err := em.em.Put(ctx, key, string(largeValue))
			assert.NoError(t, err)

			got, err := em.em.Get(ctx, key)
			assert.NoError(t, err)
			assert.Equal(t, string(largeValue), got)
		})
	}
}

func TestEtcdManagerErrorHandling(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			t.Run("Get Non-existent Key", func(t *testing.T) {
				_, err := em.em.Get(ctx, "non_existent_key")
				assert.Error(t, err)
			})

			t.Run("Delete Non-existent Key", func(t *testing.T) {
				err := em.em.Delete(ctx, "non_existent_key")
				assert.NoError(t, err) // Deleting a non-existent key is not an error in etcd
			})
			t.Run("Invalid Transaction", func(t *testing.T) {
				txn, err := em.em.BeginTxn(ctx, "invalid_txn")
				require.NoError(t, err)

				// Attempt to commit an empty transaction
				_, err = txn.Commit()
				assert.Error(t, err)
			})

			t.Run("Expired Context", func(t *testing.T) {
				expiredCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(1 * time.Millisecond) // Ensure context is expired

				err := em.em.Put(expiredCtx, "expired_key", "value")
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "context deadline exceeded")
			})

			t.Run("Invalid Endpoints", func(t *testing.T) {
				_, err := NewEtcdManager(
					WithMode(mode),
					WithEndpoints([]string{"invalid:12345"}),
					WithDialTimeout(100*time.Millisecond),
				)
				assert.Error(t, err)
			})
		})
	}
}

func TestEtcdManagerConcurrentTransactions(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			const numTransactions = 10
			var wg sync.WaitGroup
			wg.Add(numTransactions)

			for i := 0; i < numTransactions; i++ {
				go func(id int) {
					defer wg.Done()
					txn, err := em.em.BeginTxn(ctx, fmt.Sprintf("lock_%d", id))
					require.NoError(t, err)

					key1 := fmt.Sprintf("concurrent_tx_key1_%d", id)
					key2 := fmt.Sprintf("concurrent_tx_key2_%d", id)

					txn.Put(key1, fmt.Sprintf("value1_%d", id))
					txn.Put(key2, fmt.Sprintf("value2_%d", id))

					_, err = txn.Commit()
					assert.NoError(t, err)

					// Verify the transaction results
					val1, err := em.em.Get(ctx, key1)
					assert.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("value1_%d", id), val1)

					val2, err := em.em.Get(ctx, key2)
					assert.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("value2_%d", id), val2)
				}(i)
			}

			wg.Wait()
		})
	}
}

func TestEtcdManagerWatchPrefix(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			prefix := "watch_prefix_"
			watchChan := em.em.client.Watch(ctx, prefix, clientv3.WithPrefix())

			go func() {
				time.Sleep(100 * time.Millisecond)
				em.em.Put(ctx, prefix+"key1", "value1")
				time.Sleep(100 * time.Millisecond)
				em.em.Put(ctx, prefix+"key2", "value2")
				time.Sleep(100 * time.Millisecond)
				em.em.Delete(ctx, prefix+"key1")
			}()

			expectedEvents := []struct {
				eventType string
				key       string
				value     string
			}{
				{"PUT", prefix + "key1", "value1"},
				{"PUT", prefix + "key2", "value2"},
				{"DELETE", prefix + "key1", ""},
			}

			for _, expected := range expectedEvents {
				select {
				case watchResp := <-watchChan:
					assert.Len(t, watchResp.Events, 1)
					event := watchResp.Events[0]
					assert.Equal(t, expected.eventType, event.Type.String())
					assert.Equal(t, expected.key, string(event.Kv.Key))
					if expected.eventType != "DELETE" {
						assert.Equal(t, expected.value, string(event.Kv.Value))
					}
				case <-ctx.Done():
					t.Fatal("Watch timed out")
				}
			}
		})
	}
}

func TestEtcdManagerLeaseOperations(t *testing.T) {
	// Start a standalone etcd server for LeaderRemote and Follower tests
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"                                                  // Reduce log noise
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: "localhost:0"}} // Use any available port
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls

	etcdServer, err := embed.StartEtcd(cfg)
	require.NoError(t, err, "Failed to start embedded etcd server")
	defer etcdServer.Close()

	select {
	case <-etcdServer.Server.ReadyNotify():
		t.Log("Etcd server is ready")
	case <-time.After(10 * time.Second):
		t.Fatal("Etcd server took too long to start")
	}

	standaloneEndpoint := etcdServer.Clients[0].Addr().String()
	t.Logf("Standalone etcd server started at %s", standaloneEndpoint)

	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithDataDir(t.TempDir()), WithPorts(2382, 2381)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{standaloneEndpoint})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{standaloneEndpoint})}},
		{"Standalone", ModeStandalone, []Option{WithDataDir(t.TempDir()), WithPorts(2384, 2383)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em, err := NewEtcdManager(append(tt.opts, WithMode(tt.mode))...)
			require.NoError(t, err, "Failed to create EtcdManager")
			defer em.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Create a lease
			lease, err := em.client.Grant(ctx, 5) // 5 second lease
			require.NoError(t, err, "Failed to create lease")

			// Put a key with the lease
			key := "lease_key"
			value := "lease_value"
			_, err = em.client.Put(ctx, key, value, clientv3.WithLease(lease.ID))
			require.NoError(t, err, "Failed to put key with lease")

			// Verify the key exists
			resp, err := em.Get(ctx, key)
			assert.NoError(t, err, "Failed to get key")
			assert.Equal(t, value, resp, "Unexpected value for key")

			// Wait for the lease to expire
			select {
			case <-time.After(6 * time.Second):
				t.Log("Waited for lease to expire")
			case <-ctx.Done():
				t.Fatal("Context cancelled while waiting for lease to expire")
			}

			// Verify the key no longer exists
			_, err = em.Get(ctx, key)
			assert.Error(t, err, "Expected error when getting expired key")
			assert.Contains(t, err.Error(), "key not found", "Unexpected error message")
		})
	}
}
func TestEtcdManagerCompaction(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			// Put some data
			for i := 0; i < 10; i++ {
				key := fmt.Sprintf("compaction_key_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := em.em.Put(ctx, key, value)
				require.NoError(t, err)
			}

			// Get the current revision
			resp, err := em.em.client.Get(ctx, "compaction_key_0")
			require.NoError(t, err)
			currentRev := resp.Header.Revision

			// Compact the log
			_, err = em.em.client.Compact(ctx, currentRev)
			assert.NoError(t, err)

			// Try to get an old revision (should fail)
			_, err = em.em.client.Get(ctx, "compaction_key_0", clientv3.WithRev(currentRev-5))
			assert.Error(t, err)
		})
	}
}

func TestEtcdManagerDefragmentation(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			// Put and delete a lot of data to fragment the database
			for i := 0; i < 1000; i++ {
				key := fmt.Sprintf("defrag_key_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := em.em.Put(ctx, key, value)
				require.NoError(t, err)
				err = em.em.Delete(ctx, key)
				require.NoError(t, err)
			}

			// Defragment the database
			_, err := em.em.client.Defragment(ctx, "")
			assert.NoError(t, err)
		})
	}
}

func TestEtcdManagerAuthentication(t *testing.T) {
	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			em := newTestEtcdManager(t, mode)
			ctx := context.Background()

			// Enable authentication
			_, err := em.em.client.Auth.UserAdd(ctx, "testuser", "testpassword")
			require.NoError(t, err)

			_, err = em.em.client.Auth.RoleAdd(ctx, "testrole")
			require.NoError(t, err)

			_, err = em.em.client.Auth.UserGrantRole(ctx, "testuser", "testrole")
			require.NoError(t, err)

			_, err = em.em.client.Auth.AuthEnable(ctx)
			require.NoError(t, err)

			// Try to perform an operation without authentication (should fail)
			_, err = em.em.client.Put(ctx, "test_key", "test_value")
			assert.Error(t, err)

			// Authenticate and try again
			em.em.client.Auth = clientv3.NewAuth(em.em.client)
			_, err = em.em.client.Authenticate(ctx, "testuser", "testpassword")
			require.NoError(t, err)

			_, err = em.em.client.Put(ctx, "test_key", "test_value")
			assert.NoError(t, err)

			// Clean up: disable authentication
			_, err = em.em.client.Auth.AuthDisable(ctx)
			require.NoError(t, err)
		})
	}
}

func TestEtcdManagerRetry(t *testing.T) {
	// Start a real etcd server for testing
	etcdServer, err := NewEtcdManager(
		WithMode(ModeStandalone),
		WithDataDir(t.TempDir()),
		WithPorts(2380, 2379),
	)
	require.NoError(t, err)
	defer etcdServer.Close()

	// Create an EtcdManager client connected to the test server
	client, err := NewEtcdManager(
		WithMode(ModeFollower),
		WithEndpoints([]string{"localhost:2379"}),
		WithRetryAttempts(3),
		WithRetryDelay(100*time.Millisecond),
	)
	require.NoError(t, err)
	defer client.Close()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Simulate network issues by closing and reopening the client connection
	simulateNetworkIssue := func() {
		client.Close()
		time.Sleep(200 * time.Millisecond)
		newClient, err := clientv3.New(clientv3.Config{
			Endpoints: []string{"localhost:2379"},
		})
		require.NoError(t, err)
		client.client = newClient
	}

	// Test Put with retry
	t.Run("Put with retry", func(t *testing.T) {
		key := "retry_test_key"
		value := "retry_test_value"

		// Simulate a network issue before the operation
		simulateNetworkIssue()

		// Attempt to Put, which should trigger retries
		err := client.Put(ctx, key, value)
		assert.NoError(t, err, "Put operation should eventually succeed")

		// Verify the value was set
		gotValue, err := client.Get(ctx, key)
		assert.NoError(t, err, "Get operation should succeed")
		assert.Equal(t, value, gotValue, "Retrieved value should match the put value")
	})

	// Test Get with retry
	t.Run("Get with retry", func(t *testing.T) {
		key := "retry_get_key"
		value := "retry_get_value"

		// Set a value directly on the server
		_, err := etcdServer.client.Put(ctx, key, value)
		require.NoError(t, err, "Setting up test data should succeed")

		// Simulate a network issue before the operation
		simulateNetworkIssue()

		// Attempt to Get, which should trigger retries
		gotValue, err := client.Get(ctx, key)
		assert.NoError(t, err, "Get operation should eventually succeed")
		assert.Equal(t, value, gotValue, "Retrieved value should match the expected value")
	})
}
