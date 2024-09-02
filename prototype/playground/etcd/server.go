package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

const (
	actionListKey = "/actions/"
	responseKey   = "/responses/"
	dagKey        = "/dag"
)

type EtcdMode string

const (
	ModeEmbedded EtcdMode = "embedded"
	ModeRemote   EtcdMode = "remote"
)

type ServerConfig struct {
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

type DAGNode struct {
	ID       string   `json:"id"`
	Data     string   `json:"data"`
	Children []string `json:"children"`
}

type DAG struct {
	Nodes map[string]*DAGNode `json:"nodes"`
}

type Server struct {
	etcdServer *embed.Etcd
	etcdClient *clientv3.Client
	dag        DAG
}

func NewServer(cfg ServerConfig) (*Server, error) {
	var err error
	s := &Server{
		dag: DAG{Nodes: make(map[string]*DAGNode)},
	}

	switch cfg.Mode {
	case ModeEmbedded:
		s.etcdServer, err = startEmbeddedEtcd()
		if err != nil {
			return nil, fmt.Errorf("failed to start embedded etcd: %v", err)
		}
		s.etcdClient, err = clientv3.New(clientv3.Config{
			Endpoints:   []string{s.etcdServer.Config().ListenClientUrls[0].String()},
			DialTimeout: 5 * time.Second,
		})
	case ModeRemote:
		s.etcdClient, err = clientv3.New(clientv3.Config{
			Endpoints:   cfg.Endpoints,
			DialTimeout: 5 * time.Second,
		})
	default:
		return nil, fmt.Errorf("invalid mode: %s", cfg.Mode)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create etcd client: %v", err)
	}

	return s, nil
}

func (s *Server) Close() {
	if s.etcdClient != nil {
		s.etcdClient.Close()
	}
	if s.etcdServer != nil {
		s.etcdServer.Close()
	}
}

func (s *Server) Run() error {
	s.saveDag()
	go s.watchActions()

	http.HandleFunc("/status", s.handleStatus)
	go func() {
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	return nil
}

func startEmbeddedEtcd() (*embed.Etcd, error) {
	cfg := embed.NewConfig()
	cfg.Dir = "default.etcd"
	return embed.StartEtcd(cfg)
}

func (s *Server) watchActions() {
	watchChan := s.etcdClient.Watch(context.Background(), actionListKey, clientv3.WithPrefix())
	for watchResponse := range watchChan {
		for _, event := range watchResponse.Events {
			if event.Type == clientv3.EventTypePut {
				var action Action
				err := json.Unmarshal(event.Kv.Value, &action)
				if err != nil {
					log.Printf("Error unmarshaling action: %v", err)
					continue
				}
				go s.handleAction(action)
			}
		}
	}
}

func (s *Server) handleAction(action Action) {
	response := ActionResponse{ActionID: action.ID}

	err := s.processAction(action)
	if err != nil {
		response.Success = false
		response.Message = err.Error()
	} else {
		response.Success = true
		response.Message = "Action processed successfully"
	}

	s.sendResponse(response)
}

func (s *Server) processAction(action Action) error {
	switch action.Type {
	case "add":
		if _, exists := s.dag.Nodes[action.NodeID]; exists {
			return fmt.Errorf("node with ID %s already exists", action.NodeID)
		}
		s.dag.Nodes[action.NodeID] = &DAGNode{
			ID:       action.NodeID,
			Data:     action.Data,
			Children: []string{},
		}
	case "update":
		if node, ok := s.dag.Nodes[action.NodeID]; ok {
			node.Data = action.Data
		} else {
			return fmt.Errorf("node with ID %s does not exist", action.NodeID)
		}
	case "delete":
		if _, ok := s.dag.Nodes[action.NodeID]; ok {
			delete(s.dag.Nodes, action.NodeID)
		} else {
			return fmt.Errorf("node with ID %s does not exist", action.NodeID)
		}
	case "addChild":
		if node, ok := s.dag.Nodes[action.NodeID]; ok {
			if _, childExists := s.dag.Nodes[action.Data]; !childExists {
				return fmt.Errorf("child node with ID %s does not exist", action.Data)
			}
			node.Children = append(node.Children, action.Data)
		} else {
			return fmt.Errorf("parent node with ID %s does not exist", action.NodeID)
		}
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	s.saveDag()
	return nil
}

func (s *Server) sendResponse(response ActionResponse) {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling response: %v", err)
		return
	}

	_, err = s.etcdClient.Put(context.Background(), responseKey+response.ActionID, string(responseJSON))
	if err != nil {
		log.Printf("Error sending response to etcd: %v", err)
	}
}

func (s *Server) saveDag() {
	dagJSON, err := json.Marshal(s.dag)
	if err != nil {
		log.Printf("Error marshaling DAG: %v", err)
		return
	}
	_, err = s.etcdClient.Put(context.Background(), dagKey, string(dagJSON))
	if err != nil {
		log.Printf("Error saving DAG to etcd: %v", err)
	}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	dagJSON, err := json.Marshal(s.dag)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(dagJSON)
}

func main() {
	mode := flag.String("mode", "embedded", "etcd mode: embedded or remote")
	endpoints := flag.String("endpoints", "", "comma-separated list of etcd endpoints (for remote mode)")
	flag.Parse()

	cfg := ServerConfig{
		Mode: EtcdMode(*mode),
	}
	if *mode == "remote" {
		cfg.Endpoints = strings.Split(*endpoints, ",")
	}

	server, err := NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	if err := server.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
