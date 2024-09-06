// Package etcd provides a high-level manager for etcd operations,
// supporting various deployment modes and offering a simplified API
// for common etcd operations.
package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.etcd.io/etcd/server/v3/embed"
)

// Constants for command and response key prefixes in etcd
const (
	commandPrefix  = "/commands/"
	responsePrefix = "/responses/"
)

// Mode represents the operational mode of the EtcdManager
type Mode int

const (
	// ModeLeaderEmbed: Runs an embedded etcd server as a leader
	ModeLeaderEmbed Mode = iota
	// ModeLeaderRemote: Connects to a remote etcd server as a leader
	ModeLeaderRemote
	// ModeFollower: Connects to an etcd cluster as a follower (client)
	ModeFollower
	// ModeStandalone: Runs a standalone embedded etcd server
	ModeStandalone
	// ModeBeacon: Runs as a beacon for followers to discover the etcd endpoint
	ModeBeacon
)

func (m Mode) String() string {
	switch m {
	case ModeLeaderEmbed:
		return "LeaderEmbed"
	case ModeLeaderRemote:
		return "LeaderRemote"
	case ModeFollower:
		return "Follower"
	case ModeStandalone:
		return "Standalone"
	case ModeBeacon:
		return "Beacon"
	default:
		return "Unknown"
	}
}

// EtcdManager encapsulates the etcd client, server, and configuration
type EtcdManager struct {
	mode       Mode
	dataDir    string
	endpoints  []string
	peerPort   int
	clientPort int
	client     *clientv3.Client
	server     *embed.Etcd

	dialTimeout    time.Duration
	requestTimeout time.Duration
	retryAttempts  int
	retryDelay     time.Duration

	username     string
	password     string
	removeDarDir bool

	beaconListener net.Listener
	beaconAddr     string
}

// Option is a function type for applying configuration options to EtcdManager
type Option func(*EtcdManager) error

// NewEtcd creates and initializes a new EtcdManager with the given options
func NewEtcd(opts ...Option) (*EtcdManager, error) {
	// Initialize with default values
	em := &EtcdManager{
		mode:           ModeStandalone,
		dataDir:        "etcd_data",
		peerPort:       2380,
		clientPort:     2379,
		dialTimeout:    5 * time.Second,
		requestTimeout: 5 * time.Second,
		retryAttempts:  3,
		retryDelay:     1 * time.Second,
		removeDarDir:   false,
	}

	// Apply all provided options
	for _, opt := range opts {
		if err := opt(em); err != nil {
			return nil, err
		}
	}

	// Initialize the etcd client and/or server based on the mode
	if err := em.initialize(); err != nil {
		return nil, err
	}

	// Perform a health check
	ctx, cancel := context.WithTimeout(context.Background(), em.dialTimeout)
	defer cancel()
	_, err := em.client.Get(ctx, "health_check")
	if err != nil {
		em.Close() // Clean up resources
		return nil, fmt.Errorf("failed to connect to etcd: %v", err)
	}

	return em, nil
}

// Mode returns the current operational mode of the EtcdManager
func (em *EtcdManager) Mode() Mode {
	return em.mode
}

// initialize sets up the etcd client and/or server based on the operational mode
func (em *EtcdManager) initialize() error {
	var err error

	switch em.mode {
	case ModeLeaderEmbed, ModeStandalone:
		// Start an embedded etcd server for leader embed and standalone modes
		em.server, err = em.retryStartEmbeddedEtcd(5, 100*time.Millisecond)
		if err != nil {
			return fmt.Errorf("failed to start embedded etcd: %v", err)
		}
		em.endpoints = []string{em.server.Config().ListenClientUrls[0].String()}
	case ModeLeaderRemote, ModeFollower, ModeBeacon:
		// For remote leader and follower modes, use the provided endpoints
		if len(em.endpoints) == 0 {
			return fmt.Errorf("no endpoints provided for remote or follower mode")
		}
	}

	// Create an etcd client for all modes
	clientConfig := clientv3.Config{
		Endpoints:   em.endpoints,
		DialTimeout: em.dialTimeout,
	}

	// Add authentication if username and password are provided
	if em.username != "" && em.password != "" {
		clientConfig.Username = em.username
		clientConfig.Password = em.password
	}

	// Create an etcd client for all modes
	em.client, err = clientv3.New(clientConfig)
	if err != nil {
		if em.server != nil {
			em.server.Close()
		}
		return fmt.Errorf("failed to create etcd client: %v", err)
	}

	if em.mode == ModeBeacon {
		if err := em.startBeacon(); err != nil {
			return fmt.Errorf("failed to start beacon: %v", err)
		}
	}

	return nil
}

func (em *EtcdManager) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), em.dialTimeout)
	defer cancel()
	_, err := em.client.Get(ctx, "health_check")
	return err
}

// startEmbeddedEtcd initializes and starts an embedded etcd server
func (em *EtcdManager) startEmbeddedEtcd() (*embed.Etcd, error) {
	cfg := embed.NewConfig()
	cfg.Dir = filepath.Join(em.dataDir, fmt.Sprintf("etcd_%d_%d", em.peerPort, em.clientPort))
	cfg.ListenPeerUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("localhost:%d", em.peerPort)}}
	cfg.ListenClientUrls = []url.URL{{Scheme: "http", Host: fmt.Sprintf("localhost:%d", em.clientPort)}}
	cfg.AdvertisePeerUrls = cfg.ListenPeerUrls
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.InitialCluster = fmt.Sprintf("default=http://localhost:%d", em.peerPort)

	// Special configuration for standalone mode
	if em.mode == ModeStandalone {
		cfg.ClusterState = embed.ClusterStateFlagNew
		cfg.InitialClusterToken = "standalone-token"
	}

	return embed.StartEtcd(cfg)
}

func (em *EtcdManager) retryStartEmbeddedEtcd(maxAttempts int, initialBackoff time.Duration) (*embed.Etcd, error) {
	var server *embed.Etcd
	var err error
	backoff := initialBackoff

	for attempt := 0; attempt < maxAttempts; attempt++ {
		server, err = em.startEmbeddedEtcd()
		if err == nil {
			return server, nil
		}

		log.Printf("Attempt %d failed to start embedded etcd: %v. Retrying in %v...", attempt+1, err, backoff)
		time.Sleep(backoff)
		backoff *= 2 // Exponential backoff
	}

	return nil, fmt.Errorf("failed to start embedded etcd after %d attempts: %v", maxAttempts, err)
}

func (em *EtcdManager) startBeacon() error {
	lis, err := net.Listen("tcp", em.beaconAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on beacon address: %v", err)
	}
	em.beaconListener = lis

	go em.serveBeacon()

	return nil
}

func (em *EtcdManager) serveBeacon() {
	for {
		conn, err := em.beaconListener.Accept()
		if err != nil {
			log.Printf("Beacon accept error: %v\n", err)
			continue
		}
		go em.handleBeaconConnection(conn)
	}
}

func (em *EtcdManager) handleBeaconConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Printf("New connection from %s\n", conn.RemoteAddr())

	endpointsJSON, err := json.Marshal(em.endpoints)
	if err != nil {
		fmt.Printf("Error marshaling endpoints for %s: %v\n", conn.RemoteAddr(), err)
		return
	}

	_, err = conn.Write(endpointsJSON)
	if err != nil {
		fmt.Printf("Error sending endpoints to %s: %v\n", conn.RemoteAddr(), err)
	} else {
		fmt.Printf("Successfully sent endpoints to %s\n", conn.RemoteAddr())
	}
}

// Close gracefully shuts down the etcd client and server (if applicable)
func (em *EtcdManager) Close() {
	if em.client != nil {
		em.client.Close()
	}
	if em.server != nil {
		em.server.Close()
	}
	if em.beaconListener != nil {
		em.beaconListener.Close()
	}

	// Add a small delay to ensure all resources are released
	time.Sleep(100 * time.Millisecond)

	// Remove the data directory
	if em.removeDarDir {
		os.RemoveAll(em.dataDir)
	}
}

// OperationOptions allows customization of individual etcd operations
type OperationOptions struct {
	Timeout       time.Duration
	RetryAttempts int
	RetryDelay    time.Duration
}

// mergeOptions combines default options with any provided custom options
func (em *EtcdManager) mergeOptions(opts ...OperationOptions) OperationOptions {
	if len(opts) == 0 {
		return OperationOptions{
			Timeout:       em.requestTimeout,
			RetryAttempts: em.retryAttempts,
			RetryDelay:    em.retryDelay,
		}
	}
	return opts[0]
}

// Get retrieves the value associated with the given key
func (em *EtcdManager) Get(ctx context.Context, key string, opts ...OperationOptions) (string, error) {
	options := em.mergeOptions(opts...)
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	var result string
	err := em.retryOperation(ctx, options, func() error {
		resp, err := em.client.Get(ctx, key)
		if err != nil {
			return err
		}
		if len(resp.Kvs) == 0 {
			return fmt.Errorf("key not found: %s", key)
		}
		result = string(resp.Kvs[0].Value)
		return nil
	})
	return result, err
}

// Put sets the value for a given key
func (em *EtcdManager) Put(ctx context.Context, key, value string, opts ...OperationOptions) error {
	options := em.mergeOptions(opts...)
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	return em.retryOperation(ctx, options, func() error {
		_, err := em.client.Put(ctx, key, value)
		return err
	})
}

// Delete removes a key-value pair from etcd
func (em *EtcdManager) Delete(ctx context.Context, key string, opts ...OperationOptions) error {
	options := em.mergeOptions(opts...)
	ctx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	return em.retryOperation(ctx, options, func() error {
		_, err := em.client.Delete(ctx, key)
		return err
	})
}

// Watch sets up a watch on a specific key or prefix
func (em *EtcdManager) Watch(ctx context.Context, key string) clientv3.WatchChan {
	return em.client.Watch(ctx, key)
}

// Command represents a command to be executed by the etcd cluster
type Command struct {
	ID     string      `json:"id"`
	Type   string      `json:"type"`
	NodeID string      `json:"nodeId"`
	Data   interface{} `json:"data"`
}

// Response represents a response to a command
type Response struct {
	CommandID string `json:"commandId"`
	Success   bool   `json:"success"`
	Message   string `json:"message"`
}

// SendCommand sends a command to the etcd cluster and waits for a response
func (em *EtcdManager) SendCommand(cmd Command, opts ...OperationOptions) (<-chan Response, error) {
	options := em.mergeOptions(opts...)
	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), options.Timeout)
	defer cancel()

	log.Printf("Sending command: %+v", cmd)
	err = em.Put(ctx, commandPrefix+cmd.ID, string(cmdJSON), options)
	if err != nil {
		log.Printf("Error sending command: %v", err)
		return nil, fmt.Errorf("failed to send command to etcd: %v", err)
	}
	log.Printf("Command sent successfully")

	respChan := make(chan Response, 1)
	log.Printf("Command sent successfully, waiting for response")
	go em.waitForResponse(cmd.ID, respChan, options)
	log.Printf("Returning response channel")

	return respChan, nil
}

// waitForResponse waits for a response to a specific command
func (em *EtcdManager) waitForResponse(cmdID string, respChan chan<- Response, options OperationOptions) {
	log.Printf("Starting to wait for response for command %s", cmdID)
	ctx, cancel := context.WithTimeout(context.Background(), options.Timeout)
	defer cancel()

	watchChan := em.Watch(ctx, responsePrefix+cmdID)
	select {
	case watchResp := <-watchChan:
		log.Printf("Received watch response for command %s: %+v", cmdID, watchResp)
		for _, ev := range watchResp.Events {
			log.Printf("Event: %+v", ev)
			if ev.Type == clientv3.EventTypePut {
				var resp Response
				err := json.Unmarshal(ev.Kv.Value, &resp)
				if err != nil {
					log.Printf("Error unmarshaling response: %v", err)
					continue
				}
				log.Printf("Unmarshaled response: %+v", resp)
				respChan <- resp
				close(respChan)
				return
			}
		}
	case <-ctx.Done():
		log.Printf("Timeout waiting for response for command %s", cmdID)
		respChan <- Response{CommandID: cmdID, Success: false, Message: "Timeout waiting for response"}
		close(respChan)
	}
}

// WatchCommands sets up a watch for incoming commands
// WatchCommands sets up a watch for incoming commands
func (em *EtcdManager) WatchCommands(ctx context.Context) clientv3.WatchChan {
	log.Printf("Starting to watch commands and responses")
	return em.client.Watch(ctx, commandPrefix, clientv3.WithPrefix())
}

// SendResponse sends a response to a specific command
func (em *EtcdManager) SendResponse(resp Response, opts ...OperationOptions) error {
	log.Printf("SendResponse called with: %+v", resp)
	options := em.mergeOptions(opts...)
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), options.Timeout)
	defer cancel()

	log.Printf("Sending response: %+v", resp)
	err = em.Put(ctx, responsePrefix+resp.CommandID, string(respJSON), options)
	if err != nil {
		return fmt.Errorf("failed to send response to etcd: %v", err)
	}
	log.Printf("Response sent successfully")
	return nil
}

// retryOperation retries an operation with exponential backoff
func (em *EtcdManager) retryOperation(ctx context.Context, options OperationOptions, op func() error) error {
	var lastErr error
	for i := 0; i < options.RetryAttempts; i++ {
		select {
		case <-ctx.Done():
			// If the context is cancelled or times out, return immediately
			return ctx.Err()
		default:
			// Context is still valid, proceed with the operation
		}

		err := op()
		if err == nil {
			// Operation succeeded, return nil
			return nil
		}
		lastErr = err
		// Wait before the next retry, using the configured delay
		time.Sleep(options.RetryDelay)
	}
	// All retry attempts failed, return the last error encountered
	return fmt.Errorf("operation failed after %d attempts: %v", options.RetryAttempts, lastErr)
}

// func (em *EtcdManager) retryOperation(ctx context.Context, options OperationOptions, op interface{}) (interface{}, error) {
// 	var lastErr error
// 	for i := 0; i < options.RetryAttempts; i++ {
// 		select {
// 		case <-ctx.Done():
// 			return nil, ctx.Err()
// 		default:
// 		}

// 		switch operation := op.(type) {
// 		case func() (string, error):
// 			result, err := operation()
// 			if err == nil {
// 				return result, nil
// 			}
// 			lastErr = err
// 		case func() (*clientv3.TxnResponse, error):
// 			result, err := operation()
// 			if err == nil {
// 				return result, nil
// 			}
// 			lastErr = err
// 		default:
// 			return nil, fmt.Errorf("unsupported operation type")
// 		}

// 		time.Sleep(options.RetryDelay)
// 	}
// 	return nil, fmt.Errorf("operation failed after %d attempts: %v", options.RetryAttempts, lastErr)
// }

func (em *EtcdManager) retryOperationTx(ctx context.Context, options OperationOptions, operation func() (*clientv3.TxnResponse, error)) (*clientv3.TxnResponse, error) {
	var lastErr error
	for i := 0; i < options.RetryAttempts; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		result, err := operation()
		if err == nil {
			return result, nil
		}
		lastErr = err

		time.Sleep(options.RetryDelay)
	}
	return nil, fmt.Errorf("operation failed after %d attempts: %v", options.RetryAttempts, lastErr)
}

type TxnOp struct {
	Key   string
	Value string
	Type  string // "put", "delete", or "get"
}

type EtcdTxn struct {
	ctx     context.Context
	txn     clientv3.Txn
	client  *clientv3.Client
	mutex   *concurrency.Mutex
	ops     []clientv3.Op
	manager *EtcdManager
	options OperationOptions
}

func (em *EtcdManager) BeginTxn(ctx context.Context, lockName string, opts ...OperationOptions) (*EtcdTxn, error) {
	options := em.mergeOptions(opts...)
	session, err := concurrency.NewSession(em.client)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}

	mutex := concurrency.NewMutex(session, fmt.Sprintf("/locks/%s", lockName))

	if err := mutex.Lock(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %v", err)
	}

	return &EtcdTxn{
		ctx:     ctx,
		txn:     em.client.Txn(ctx),
		client:  em.client,
		mutex:   mutex,
		ops:     []clientv3.Op{},
		manager: em,
		options: options,
	}, nil
}

func (et *EtcdTxn) Put(key, value string) *EtcdTxn {
	et.ops = append(et.ops, clientv3.OpPut(key, value))
	return et
}

func (et *EtcdTxn) Delete(key string) *EtcdTxn {
	et.ops = append(et.ops, clientv3.OpDelete(key))
	return et
}

func (et *EtcdTxn) Get(key string) *EtcdTxn {
	et.ops = append(et.ops, clientv3.OpGet(key))
	return et
}

func (et *EtcdTxn) Commit() (*clientv3.TxnResponse, error) {
	defer et.mutex.Unlock(et.ctx)
	return et.manager.retryOperationTx(et.ctx, et.options, func() (*clientv3.TxnResponse, error) {
		return et.txn.Then(et.ops...).Commit()
	})
}

func (et *EtcdTxn) Rollback() error {
	defer et.mutex.Unlock(et.ctx)
	// In etcd, if we don't commit, the transaction is automatically rolled back
	// So we just need to release the lock
	return nil
}

func (em *EtcdManager) ExecuteTransaction(ctx context.Context, lockName string, ops []TxnOp, opts ...OperationOptions) (*clientv3.TxnResponse, error) {
	txn, err := em.BeginTxn(ctx, lockName, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}

	for _, op := range ops {
		switch op.Type {
		case "put":
			txn.Put(op.Key, op.Value)
		case "delete":
			txn.Delete(op.Key)
		case "get":
			txn.Get(op.Key)
		default:
			txn.Rollback()
			return nil, fmt.Errorf("unsupported operation type: %s", op.Type)
		}
	}

	return txn.Commit()
}

// Option functions

// WithMode sets the operational mode of the EtcdManager
func WithMode(mode Mode) Option {
	return func(em *EtcdManager) error {
		em.mode = mode
		return nil
	}
}

// WithDataDir sets the data directory for the embedded etcd server
func WithDataDir(dir string) Option {
	return func(em *EtcdManager) error {
		// Ensure the directory exists
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create data directory: %v", err)
		}
		em.dataDir = dir
		return nil
	}
}

// WithEndpoints sets the etcd endpoints for remote connections
func WithEndpoints(endpoints []string) Option {
	return func(em *EtcdManager) error {
		if len(endpoints) == 0 {
			return fmt.Errorf("at least one endpoint is required")
		}
		em.endpoints = endpoints
		return nil
	}
}

// WithPorts sets the peer and client ports for the embedded etcd server
func WithPorts(peerPort, clientPort int) Option {
	return func(em *EtcdManager) error {
		em.peerPort = peerPort
		em.clientPort = clientPort
		return nil
	}
}

// WithBeacon sets up the EtcdManager as a beacon
func WithBeacon(addr string) Option {
	return func(em *EtcdManager) error {
		em.mode = ModeBeacon
		em.beaconAddr = addr
		return nil
	}
}

// WithDialTimeout sets the dial timeout for etcd client connections
func WithDialTimeout(timeout time.Duration) Option {
	return func(em *EtcdManager) error {
		em.dialTimeout = timeout
		return nil
	}
}

// WithRequestTimeout sets the default timeout for etcd operations
func WithRequestTimeout(timeout time.Duration) Option {
	return func(em *EtcdManager) error {
		em.requestTimeout = timeout
		return nil
	}
}

// WithRetryAttempts sets the number of retry attempts for operations
func WithRetryAttempts(attempts int) Option {
	return func(em *EtcdManager) error {
		if attempts < 1 {
			return fmt.Errorf("retry attempts must be at least 1")
		}
		em.retryAttempts = attempts
		return nil
	}
}

// WithRetryDelay sets the delay between retry attempts
func WithRetryDelay(delay time.Duration) Option {
	return func(em *EtcdManager) error {
		em.retryDelay = delay
		return nil
	}
}

// WithAuth sets the username and password for etcd authentication
func WithAuth(username, password string) Option {
	return func(em *EtcdManager) error {
		em.username = username
		em.password = password
		return nil
	}
}

// WithRemoveDataDir sets the flag to remove the data directory on close
func WithRemoveDataDir() Option {
	return func(em *EtcdManager) error {
		em.removeDarDir = true
		return nil
	}
}
