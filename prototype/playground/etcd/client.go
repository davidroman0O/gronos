package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const (
	actionListKey = "/actions/"
	responseKey   = "/responses/"
)

type EtcdMode string

const (
	ModeStandalone EtcdMode = "standalone"
	ModeEmbedded   EtcdMode = "embedded"
	ModeRemote     EtcdMode = "remote"
)

type ClientConfig struct {
	Mode      EtcdMode
	Endpoints []string
}

type Action struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	NodeID string `json:"nodeId"`
	Data   string `json:"data"`
}

type ActionResponse struct {
	ActionID string `json:"actionId"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

type DAGClient struct {
	etcdClient *clientv3.Client
	etcdServer *embed.Etcd
	mu         sync.Mutex
	pending    map[string]chan ActionResponse
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewDAGClient(cfg ClientConfig) (*DAGClient, error) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &DAGClient{
		pending: make(map[string]chan ActionResponse),
		ctx:     ctx,
		cancel:  cancel,
	}

	var err error
	var endpoints []string

	switch cfg.Mode {
	case ModeStandalone, ModeEmbedded:
		client.etcdServer, endpoints, err = startEmbeddedEtcd()
		if err != nil {
			return nil, fmt.Errorf("failed to start embedded etcd: %v", err)
		}
	case ModeRemote:
		endpoints = cfg.Endpoints
	default:
		return nil, fmt.Errorf("invalid mode: %s", cfg.Mode)
	}

	client.etcdClient, err = connectWithRetry(endpoints, 30*time.Second)
	if err != nil {
		if client.etcdServer != nil {
			client.etcdServer.Close()
		}
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}

	go client.watchResponses()

	return client, nil
}

func connectWithRetry(endpoints []string, timeout time.Duration) (*clientv3.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var client *clientv3.Client
	var err error

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout while connecting to etcd: %v", err)
		default:
			client, err = clientv3.New(clientv3.Config{
				Endpoints:   endpoints,
				DialTimeout: 5 * time.Second,
			})
			if err == nil {
				return client, nil
			}
			log.Printf("Failed to connect to etcd, retrying: %v", err)
			time.Sleep(1 * time.Second)
		}
	}
}

func startEmbeddedEtcd() (*embed.Etcd, []string, error) {
	cfg := embed.NewConfig()
	cfg.Dir = "client_etcd_data"
	os.RemoveAll(cfg.Dir) // Clean up any existing data

	// Use random available ports
	clientPort, err := getAvailablePort()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get available client port: %v", err)
	}
	peerPort, err := getAvailablePort()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get available peer port: %v", err)
	}

	clientURL := fmt.Sprintf("http://localhost:%d", clientPort)
	peerURL := fmt.Sprintf("http://localhost:%d", peerPort)

	cfg.ListenClientUrls, _ = urlsFromStrings([]string{clientURL})
	cfg.AdvertiseClientUrls = cfg.ListenClientUrls
	cfg.ListenPeerUrls, _ = urlsFromStrings([]string{peerURL})
	cfg.InitialCluster = fmt.Sprintf("default=%s", peerURL)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		return nil, nil, err
	}

	select {
	case <-e.Server.ReadyNotify():
		log.Printf("Embedded etcd server is ready!")
	case <-time.After(10 * time.Second):
		e.Server.Stop() // trigger a shutdown
		return nil, nil, fmt.Errorf("embedded etcd server took too long to start")
	}

	return e, []string{clientURL}, nil
}

func urlsFromStrings(strs []string) ([]url.URL, error) {
	urls := make([]url.URL, len(strs))
	for i, s := range strs {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		urls[i] = *u
	}
	return urls, nil
}

func getAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (c *DAGClient) Close() {
	c.cancel()
	if c.etcdServer != nil {
		c.etcdServer.Close()
	}
	if c.etcdClient != nil {
		c.etcdClient.Close()
	}
}

func (c *DAGClient) SendAction(action Action) <-chan ActionResponse {
	respChan := make(chan ActionResponse, 1)

	c.mu.Lock()
	c.pending[action.ID] = respChan
	c.mu.Unlock()

	go func() {
		actionJSON, err := json.Marshal(action)
		if err != nil {
			c.mu.Lock()
			delete(c.pending, action.ID)
			c.mu.Unlock()
			respChan <- ActionResponse{
				ActionID: action.ID,
				Success:  false,
				Message:  fmt.Sprintf("Error marshaling action: %v", err),
			}
			close(respChan)
			return
		}

		_, err = c.etcdClient.Put(c.ctx, actionListKey+action.ID, string(actionJSON))
		if err != nil {
			c.mu.Lock()
			delete(c.pending, action.ID)
			c.mu.Unlock()
			respChan <- ActionResponse{
				ActionID: action.ID,
				Success:  false,
				Message:  fmt.Sprintf("Error sending action to etcd: %v", err),
			}
			close(respChan)
			return
		}
	}()

	return respChan
}

func (c *DAGClient) watchResponses() {
	watchChan := c.etcdClient.Watch(c.ctx, responseKey, clientv3.WithPrefix())
	for watchResponse := range watchChan {
		for _, event := range watchResponse.Events {
			if event.Type == clientv3.EventTypePut {
				var response ActionResponse
				err := json.Unmarshal(event.Kv.Value, &response)
				if err != nil {
					log.Printf("Error unmarshaling response: %v", err)
					continue
				}

				c.mu.Lock()
				if respChan, ok := c.pending[response.ActionID]; ok {
					respChan <- response
					close(respChan)
					delete(c.pending, response.ActionID)
				}
				c.mu.Unlock()
			}
		}
	}
}

func main() {
	mode := flag.String("mode", "standalone", "etcd mode: standalone, embedded, or remote")
	endpoints := flag.String("endpoints", "", "comma-separated list of etcd endpoints (for remote mode)")
	flag.Parse()

	cfg := ClientConfig{
		Mode: EtcdMode(*mode),
	}
	if *mode == "remote" {
		cfg.Endpoints = strings.Split(*endpoints, ",")
	}

	client, err := NewDAGClient(cfg)
	if err != nil {
		log.Fatalf("Failed to create DAG client: %v", err)
	}
	defer client.Close()

	// Example actions
	actions := []Action{
		{ID: "1", Type: "add", NodeID: "1", Data: "Node 1"},
		{ID: "2", Type: "add", NodeID: "2", Data: "Node 2"},
		{ID: "3", Type: "addChild", NodeID: "1", Data: "2"},
		{ID: "4", Type: "update", NodeID: "2", Data: "Updated Node 2"},
		{ID: "5", Type: "delete", NodeID: "2"},
	}

	// Send actions and wait for responses
	for _, action := range actions {
		respChan := client.SendAction(action)
		select {
		case response := <-respChan:
			fmt.Printf("Received response for action %s: Success=%v, Message=%s\n",
				response.ActionID, response.Success, response.Message)
		case <-time.After(5 * time.Second):
			fmt.Printf("Timeout waiting for response to action %s\n", action.ID)
		}
	}

	// Print the final status
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := client.etcdClient.Get(ctx, "/dag")
	if err != nil {
		log.Printf("Failed to get DAG status: %v", err)
	} else if len(resp.Kvs) > 0 {
		fmt.Printf("Final DAG status: %s\n", resp.Kvs[0].Value)
	} else {
		fmt.Println("DAG is empty")
	}
}
