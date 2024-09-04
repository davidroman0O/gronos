package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

var (
	testEtcdServer *embed.Etcd
	etcdEndpoint   string
)

func setupTestEtcd(t *testing.T) {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: "localhost:0"}}
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls

	var err error
	testEtcdServer, err = embed.StartEtcd(cfg)
	require.NoError(t, err, "Failed to start embedded etcd server")

	select {
	case <-testEtcdServer.Server.ReadyNotify():
		t.Log("Etcd server is ready")
	case <-time.After(10 * time.Second):
		t.Fatal("Etcd server took too long to start")
	}

	etcdEndpoint = testEtcdServer.Clients[0].Addr().String()
	t.Logf("Standalone etcd server started at %s", etcdEndpoint)
}

func teardownTestEtcd() {
	if testEtcdServer != nil {
		testEtcdServer.Close()
	}
}

func TestEtcdManagerInitialization(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithDataDir(t.TempDir()), WithPorts(2380, 2379)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{etcdEndpoint})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{etcdEndpoint})}},
		{"Standalone", ModeStandalone, []Option{WithDataDir(t.TempDir()), WithPorts(2382, 2381)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em, err := NewEtcdManager(append(tt.opts, WithMode(tt.mode))...)
			if err != nil {
				t.Fatalf("Failed to create EtcdManager: %v", err)
			}
			defer em.Close()

			assert.NotNil(t, em, "EtcdManager should be created successfully")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err = em.Put(ctx, "test_key", "test_value")
			assert.NoError(t, err, "Failed to put test key-value pair")

			value, err := em.Get(ctx, "test_key")
			assert.NoError(t, err, "Failed to get test key")
			assert.Equal(t, "test_value", value, "Retrieved value doesn't match the put value")
		})
	}
}

func TestEtcdManagerCRUD(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithDataDir(t.TempDir()), WithPorts(2382, 2381)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{etcdEndpoint})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{etcdEndpoint})}},
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
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
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
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx := context.Background()

			t.Run("Successful Transaction", func(t *testing.T) {
				txn, err := em.BeginTxn(ctx, "test_lock")
				require.NoError(t, err)

				txn.Put("key1", "value1")
				txn.Put("key2", "value2")

				resp, err := txn.Commit()
				assert.NoError(t, err)
				assert.True(t, resp.Succeeded)

				value1, err := em.Get(ctx, "key1")
				assert.NoError(t, err)
				assert.Equal(t, "value1", value1)

				value2, err := em.Get(ctx, "key2")
				assert.NoError(t, err)
				assert.Equal(t, "value2", value2)
			})

			t.Run("Transaction Rollback", func(t *testing.T) {
				txn, err := em.BeginTxn(ctx, "test_lock")
				require.NoError(t, err)

				txn.Put("key3", "value3")
				txn.Put("key4", "value4")

				err = txn.Rollback()
				assert.NoError(t, err)

				_, err = em.Get(ctx, "key3")
				assert.Error(t, err, "Key should not exist after rollback")

				_, err = em.Get(ctx, "key4")
				assert.Error(t, err, "Key should not exist after rollback")
			})
		})
	}
}

func TestEtcdManagerCommandResponse(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			watchChan := em.WatchCommands(ctx)
			time.Sleep(1 * time.Second) // Give some time for the watch to be set up

			cmd := Command{
				ID:     "test_cmd",
				Type:   "test",
				NodeID: "node1",
				Data:   map[string]string{"key": "value"},
			}

			respChan, err := em.SendCommand(cmd)
			require.NoError(t, err)

			// Send the response
			go func() {
				time.Sleep(2 * time.Second) // Simulate some processing time
				response := Response{
					CommandID: cmd.ID,
					Success:   true,
					Message:   "Command processed successfully",
				}
				err := em.SendResponse(response)
				require.NoError(t, err)
			}()

			// Wait for command response
			select {
			case resp := <-respChan:
				t.Logf("Received command response: %+v", resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "Command processed successfully", resp.Message)
			case <-time.After(10 * time.Second):
				t.Fatal("Timeout waiting for command response")
			}

			// Wait for watch events
			watchEventReceived := make(chan struct{})
			go func() {
				commandSeen := false
				responseSeen := false
				for watchResp := range watchChan {
					for _, ev := range watchResp.Events {
						t.Logf("Received watch event: Type=%s, Key=%s, Value=%s", ev.Type, string(ev.Kv.Key), string(ev.Kv.Value))
						if strings.HasPrefix(string(ev.Kv.Key), commandPrefix) {
							var receivedCmd map[string]interface{}
							err := json.Unmarshal(ev.Kv.Value, &receivedCmd)
							require.NoError(t, err)

							assert.Equal(t, cmd.ID, receivedCmd["id"])
							assert.Equal(t, cmd.Type, receivedCmd["type"])
							assert.Equal(t, cmd.NodeID, receivedCmd["nodeId"])

							data, ok := receivedCmd["data"].(map[string]interface{})
							assert.True(t, ok, "Data should be a map")
							assert.Equal(t, "value", data["key"])

							commandSeen = true
						} else if strings.HasPrefix(string(ev.Kv.Key), responsePrefix) {
							var receivedResp Response
							err := json.Unmarshal(ev.Kv.Value, &receivedResp)
							require.NoError(t, err)
							assert.Equal(t, cmd.ID, receivedResp.CommandID)
							assert.True(t, receivedResp.Success)
							assert.Equal(t, "Command processed successfully", receivedResp.Message)
							responseSeen = true
						}
						if commandSeen && responseSeen {
							close(watchEventReceived)
							return
						}
					}
				}
			}()

			select {
			case <-watchEventReceived:
				t.Log("Received expected watch events")
			case <-time.After(10 * time.Second):
				t.Error("Timed out waiting for watch events")
			}
		})
	}
}

func TestEtcdManagerMultipleNodes(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	leader, err := NewEtcdManager(
		WithMode(ModeLeaderRemote),
		WithEndpoints([]string{etcdEndpoint}),
	)
	require.NoError(t, err)
	defer leader.Close()

	follower1, err := NewEtcdManager(
		WithMode(ModeFollower),
		WithEndpoints([]string{etcdEndpoint}),
	)
	require.NoError(t, err)
	defer follower1.Close()

	follower2, err := NewEtcdManager(
		WithMode(ModeFollower),
		WithEndpoints([]string{etcdEndpoint}),
	)
	require.NoError(t, err)
	defer follower2.Close()

	ctx := context.Background()

	key := "multi_node_key"
	value := "multi_node_value"

	err = leader.Put(ctx, key, value)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	value1, err := follower1.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, value, value1)

	value2, err := follower2.Get(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, value, value2)
}

func TestEtcdManagerConcurrency(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

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

						err := em.Put(ctx, key, value)
						assert.NoError(t, err)

						got, err := em.Get(ctx, key)
						assert.NoError(t, err)
						assert.Equal(t, value, got)

						err = em.Delete(ctx, key)
						assert.NoError(t, err)
					}
				}(i)
			}

			wg.Wait()
		})
	}
}

func TestEtcdManagerLargeValues(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx := context.Background()

			largeValue := make([]byte, 1024*1024) // 1MB
			for i := range largeValue {
				largeValue[i] = byte(i % 256)
			}

			key := "large_value_key"
			err = em.Put(ctx, key, string(largeValue))
			assert.NoError(t, err)

			got, err := em.Get(ctx, key)
			assert.NoError(t, err)
			assert.Equal(t, string(largeValue), got)
		})
	}
}

func TestEtcdManagerErrorHandling(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx := context.Background()

			t.Run("Get Non-existent Key", func(t *testing.T) {
				_, err := em.Get(ctx, "non_existent_key")
				assert.Error(t, err)
			})

			t.Run("Delete Non-existent Key", func(t *testing.T) {
				err := em.Delete(ctx, "non_existent_key")
				assert.NoError(t, err) // Deleting a non-existent key is not an error in etcd
			})

			t.Run("Invalid Transaction", func(t *testing.T) {
				txn, err := em.BeginTxn(ctx, "invalid_txn")
				require.NoError(t, err)

				// Attempt to commit an empty transaction
				_, err = txn.Commit()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "cannot commit an empty transaction")
			})

			t.Run("Expired Context", func(t *testing.T) {
				expiredCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(1 * time.Millisecond) // Ensure context is expired

				err := em.Put(expiredCtx, "expired_key", "value")
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "context deadline exceeded")
			})
		})
	}

	t.Run("Invalid Endpoints", func(t *testing.T) {
		_, err := NewEtcdManager(
			WithMode(ModeFollower),
			WithEndpoints([]string{"invalid:12345"}),
			WithDialTimeout(100*time.Millisecond),
		)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect to etcd")
	})
}

func TestEtcdManagerConcurrentTransactions(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx := context.Background()

			const numTransactions = 10
			var wg sync.WaitGroup
			wg.Add(numTransactions)

			for i := 0; i < numTransactions; i++ {
				go func(id int) {
					defer wg.Done()
					txn, err := em.BeginTxn(ctx, fmt.Sprintf("lock_%d", id))
					require.NoError(t, err)

					key1 := fmt.Sprintf("concurrent_tx_key1_%d", id)
					key2 := fmt.Sprintf("concurrent_tx_key2_%d", id)

					txn.Put(key1, fmt.Sprintf("value1_%d", id))
					txn.Put(key2, fmt.Sprintf("value2_%d", id))

					_, err = txn.Commit()
					assert.NoError(t, err)

					// Verify the transaction results
					val1, err := em.Get(ctx, key1)
					assert.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("value1_%d", id), val1)

					val2, err := em.Get(ctx, key2)
					assert.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("value2_%d", id), val2)
				}(i)
			}

			wg.Wait()
		})
	}
}

func TestEtcdManagerWatchPrefix(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			prefix := "watch_prefix_"
			watchChan := em.client.Watch(ctx, prefix, clientv3.WithPrefix())

			go func() {
				time.Sleep(100 * time.Millisecond)
				em.Put(ctx, prefix+"key1", "value1")
				time.Sleep(100 * time.Millisecond)
				em.Put(ctx, prefix+"key2", "value2")
				time.Sleep(100 * time.Millisecond)
				em.Delete(ctx, prefix+"key1")
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
	setupTestEtcd(t)
	defer teardownTestEtcd()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
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
	setupTestEtcd(t)
	defer teardownTestEtcd()

	tests := []struct {
		name string
		mode Mode
		opts []Option
	}{
		{"LeaderEmbed", ModeLeaderEmbed, []Option{WithDataDir(t.TempDir()), WithPorts(2382, 2381)}},
		{"LeaderRemote", ModeLeaderRemote, []Option{WithEndpoints([]string{etcdEndpoint})}},
		{"Follower", ModeFollower, []Option{WithEndpoints([]string{etcdEndpoint})}},
		{"Standalone", ModeStandalone, []Option{WithDataDir(t.TempDir()), WithPorts(2384, 2383)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			em, err := NewEtcdManager(append(tt.opts, WithMode(tt.mode))...)
			require.NoError(t, err, "Failed to create EtcdManager")
			defer em.Close()

			ctx := context.Background()

			// Put some data
			for i := 0; i < 10; i++ {
				key := fmt.Sprintf("compaction_key_%d", i)
				value := fmt.Sprintf("value_%d", i)
				err := em.Put(ctx, key, value)
				require.NoError(t, err)
			}

			// Get the current revision
			resp, err := em.client.Get(ctx, "compaction_key_0")
			require.NoError(t, err)
			currentRev := resp.Header.Revision

			// Compact the log
			_, err = em.client.Compact(ctx, currentRev)
			assert.NoError(t, err)

			// Try to get an old revision (should fail)
			_, err = em.client.Get(ctx, "compaction_key_0", clientv3.WithRev(currentRev-5))
			assert.Error(t, err)
		})
	}
}

func TestEtcdManagerDefragmentation(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	t.Log("Starting TestEtcdManagerDefragmentation")

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			t.Logf("Testing mode: %s", mode)

			var em *EtcdManager
			var err error

			// Create EtcdManager based on mode
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
					WithEndpoints([]string{etcdEndpoint}),
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

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Put some data
			t.Log("Inserting test data")
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("test_key_%d", i)
				value := fmt.Sprintf("test_value_%d", i)
				err := em.Put(ctx, key, value)
				require.NoError(t, err, "Failed to put test data")
			}

			// Perform defragmentation
			t.Log("Starting defragmentation")
			for _, endpoint := range em.endpoints {
				t.Logf("Defragmenting endpoint: %s", endpoint)
				_, err = em.client.Maintenance.Defragment(ctx, endpoint)
				if err != nil {
					t.Fatalf("Defragmentation failed for endpoint %s: %v", endpoint, err)
				}
			}
			t.Log("Defragmentation completed successfully")

			// Verify data after defragmentation
			t.Log("Verifying data after defragmentation")
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("test_key_%d", i)
				expectedValue := fmt.Sprintf("test_value_%d", i)
				value, err := em.Get(ctx, key)
				require.NoError(t, err, "Failed to get test data after defragmentation")
				require.Equal(t, expectedValue, value, "Unexpected value after defragmentation")
			}

			t.Logf("Test completed successfully for mode: %s", mode)
		})
	}

	t.Log("TestEtcdManagerDefragmentation completed")
}
func TestEtcdManagerAuthentication(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	// Create a client for the standalone etcd server
	standaloneClient, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdEndpoint},
		DialTimeout: 5 * time.Second,
	})
	require.NoError(t, err)
	defer standaloneClient.Close()

	ctx := context.Background()

	modes := []Mode{ModeLeaderEmbed, ModeLeaderRemote, ModeFollower, ModeStandalone}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			// Disable authentication on the standalone etcd server before each test
			authStatus, err := standaloneClient.Auth.AuthStatus(ctx)
			require.NoError(t, err)
			if authStatus.Enabled {
				_, err = standaloneClient.Auth.AuthDisable(ctx)
				require.NoError(t, err)
			}

			var em *EtcdManager

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
					WithEndpoints([]string{etcdEndpoint}),
				)
			case ModeStandalone:
				em, err = NewEtcdManager(
					WithMode(mode),
					WithDataDir(t.TempDir()),
					WithPorts(2384, 2383),
				)
			}

			require.NoError(t, err)
			defer em.Close()

			// Create a client config without authentication
			cfg := clientv3.Config{
				Endpoints:   em.endpoints,
				DialTimeout: em.dialTimeout,
			}

			// Create an unauthenticated client
			client, err := clientv3.New(cfg)
			require.NoError(t, err, "Failed to create etcd client")
			defer client.Close()

			// Check if root user exists
			_, err = client.Auth.UserGet(ctx, "root")
			if err != nil {
				// Create root user if it doesn't exist
				_, err = client.Auth.UserAdd(ctx, "root", "rootpassword")
				require.NoError(t, err, "Failed to create root user")

				// Grant root role to root user
				_, err = client.Auth.UserGrantRole(ctx, "root", "root")
				require.NoError(t, err, "Failed to grant root role to root user")
			}

			// Enable authentication
			_, err = client.Auth.AuthEnable(ctx)
			require.NoError(t, err, "Failed to enable authentication")

			// Create a new client with root credentials
			cfg.Username = "root"
			cfg.Password = "rootpassword"
			rootClient, err := clientv3.New(cfg)
			require.NoError(t, err, "Failed to create authenticated root client")
			defer rootClient.Close()

			// Check if test user exists
			_, err = rootClient.Auth.UserGet(ctx, "testuser")
			if err != nil {
				// Use the authenticated root client for subsequent operations
				_, err = rootClient.Auth.UserAdd(ctx, "testuser", "testpassword")
				require.NoError(t, err, "Failed to add test user")

				_, err = rootClient.Auth.RoleAdd(ctx, "testrole")
				require.NoError(t, err, "Failed to add test role")

				// Grant read and write permissions to testrole for a specific key range
				_, err = rootClient.Auth.RoleGrantPermission(ctx, "testrole", "test", "zzzz", clientv3.PermissionType(clientv3.PermReadWrite))
				require.NoError(t, err, "Failed to grant permissions to test role")

				_, err = rootClient.Auth.UserGrantRole(ctx, "testuser", "testrole")
				require.NoError(t, err, "Failed to grant role to test user")
			}

			// Verify that authentication is working
			kv := clientv3.NewKV(rootClient)
			_, err = kv.Put(ctx, "test_key", "test_value")
			require.NoError(t, err, "Failed to put test key-value pair")

			// Try to access without authentication (should fail)
			_, err = clientv3.NewKV(client).Put(ctx, "unauthorized_key", "unauthorized_value")
			require.Error(t, err, "Unauthorized put should fail")

			// Verify that the test user can access with correct credentials
			cfg.Username = "testuser"
			cfg.Password = "testpassword"
			testUserClient, err := clientv3.New(cfg)
			require.NoError(t, err, "Failed to create test user client")
			defer testUserClient.Close()

			// Test within the permitted range
			_, err = clientv3.NewKV(testUserClient).Put(ctx, "test_user_key", "testuser_value")
			require.NoError(t, err, "Test user should be able to put key-value pair within permitted range")

			// Verify that the test user can read the key-value pair
			resp, err := clientv3.NewKV(testUserClient).Get(ctx, "test_user_key")
			require.NoError(t, err, "Test user should be able to get key-value pair")
			require.Equal(t, 1, len(resp.Kvs), "Expected one key-value pair")
			require.Equal(t, "testuser_value", string(resp.Kvs[0].Value), "Unexpected value for test_user_key")

			// Test outside the permitted range (should fail)
			_, err = clientv3.NewKV(testUserClient).Put(ctx, "zzzzz_key", "outside_value")
			require.Error(t, err, "Test user should not be able to put key-value pair outside permitted range")

			// Disable authentication after each test
			_, err = rootClient.Auth.AuthDisable(ctx)
			require.NoError(t, err, "Failed to disable authentication")
		})
	}
}

func TestEtcdManagerRetry(t *testing.T) {
	setupTestEtcd(t)
	defer teardownTestEtcd()

	// Create an EtcdManager client connected to the test server
	client, err := NewEtcdManager(
		WithMode(ModeFollower),
		WithEndpoints([]string{etcdEndpoint}),
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
			Endpoints: []string{etcdEndpoint},
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

		// Set a value directly on the server using a separate client
		setupClient, err := clientv3.New(clientv3.Config{
			Endpoints: []string{etcdEndpoint},
		})
		require.NoError(t, err)
		defer setupClient.Close()

		_, err = setupClient.Put(ctx, key, value)
		require.NoError(t, err, "Setting up test data should succeed")

		// Simulate a network issue before the operation
		simulateNetworkIssue()

		// Attempt to Get, which should trigger retries
		gotValue, err := client.Get(ctx, key)
		assert.NoError(t, err, "Get operation should eventually succeed")
		assert.Equal(t, value, gotValue, "Retrieved value should match the expected value")
	})
}
