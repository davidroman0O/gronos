package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/davidroman0O/gronos/dag"
	"github.com/davidroman0O/gronos/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	graphPrefix = "/graph/"
	nodePrefix  = graphPrefix + "nodes/"
	edgePrefix  = graphPrefix + "edges/"
)

type GraphManager struct {
	etcdManager  *etcd.EtcdManager
	graph        *dag.AcyclicGraph
	processID    int
	macAddress   string
	mu           sync.RWMutex
	nodeLastSeen map[string]time.Time
	mode         etcd.Mode
}

func NewGraphManager() (*GraphManager, error) {
	processID := os.Getpid()
	macAddress, err := getMacAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get MAC address: %v", err)
	}
	return &GraphManager{
		graph:        &dag.AcyclicGraph{},
		processID:    processID,
		macAddress:   macAddress,
		nodeLastSeen: make(map[string]time.Time),
	}, nil
}

func (gm *GraphManager) Run(ctx context.Context, etcdManager *etcd.EtcdManager) error {
	gm.etcdManager = etcdManager
	gm.mode = etcdManager.Mode()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	if gm.mode == etcd.ModeFollower {
		if err := gm.syncGraphFromEtcd(ctx); err != nil {
			return fmt.Errorf("failed to sync graph from etcd: %v", err)
		}
		go gm.watchGraphChanges(ctx)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			switch gm.mode {
			case etcd.ModeLeaderEmbed, etcd.ModeLeaderRemote:
				gm.cleanupStaleNodes()
			case etcd.ModeFollower:
				if err := gm.sendHeartbeat(ctx); err != nil {
					fmt.Printf("Error sending heartbeat: %v\n", err)
				}
			case etcd.ModeStandalone:
				gm.cleanupStaleNodes()
				if err := gm.sendHeartbeat(ctx); err != nil {
					fmt.Printf("Error sending heartbeat: %v\n", err)
				}
			}
		}
	}
}

func (gm *GraphManager) HandleCommand(ctx context.Context, cmd etcd.Command) (bool, error) {
	switch gm.mode {
	case etcd.ModeLeaderEmbed, etcd.ModeLeaderRemote, etcd.ModeStandalone:
		return gm.handleLeaderCommand(ctx, cmd)
	case etcd.ModeFollower:
		return false, nil // Followers don't handle commands directly
	default:
		return false, fmt.Errorf("unknown mode: %v", gm.mode)
	}
}

func (gm *GraphManager) handleLeaderCommand(ctx context.Context, cmd etcd.Command) (bool, error) {
	switch cmd.Type {
	case "graph_add_node":
		return gm.handleAddNode(ctx, cmd)
	case "graph_remove_node":
		return gm.handleRemoveNode(ctx, cmd)
	case "graph_add_edge":
		return gm.handleAddEdge(ctx, cmd)
	case "graph_remove_edge":
		return gm.handleRemoveEdge(ctx, cmd)
	case "graph_heartbeat":
		return gm.handleHeartbeat(ctx, cmd)
	default:
		return false, nil
	}
}

func (gm *GraphManager) handleAddNode(ctx context.Context, cmd etcd.Command) (bool, error) {
	var nodeData struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(cmd.Data.(string)), &nodeData); err != nil {
		return false, fmt.Errorf("failed to unmarshal node data: %v", err)
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.graph.Add(nodeData.ID)
	gm.nodeLastSeen[nodeData.ID] = time.Now()

	// Persist to etcd
	if err := gm.persistNode(ctx, nodeData.ID); err != nil {
		return false, fmt.Errorf("failed to persist node: %v", err)
	}

	return true, nil
}

func (gm *GraphManager) handleRemoveNode(ctx context.Context, cmd etcd.Command) (bool, error) {
	var nodeData struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(cmd.Data.(string)), &nodeData); err != nil {
		return false, fmt.Errorf("failed to unmarshal node data: %v", err)
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.graph.Remove(nodeData.ID)
	delete(gm.nodeLastSeen, nodeData.ID)

	// Remove from etcd
	if err := gm.removeNodeFromEtcd(ctx, nodeData.ID); err != nil {
		return false, fmt.Errorf("failed to remove node from etcd: %v", err)
	}

	return true, nil
}

func (gm *GraphManager) handleAddEdge(ctx context.Context, cmd etcd.Command) (bool, error) {
	var edgeData struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal([]byte(cmd.Data.(string)), &edgeData); err != nil {
		return false, fmt.Errorf("failed to unmarshal edge data: %v", err)
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.graph.Connect(dag.BasicEdge(edgeData.Source, edgeData.Target))

	// Persist to etcd
	if err := gm.persistEdge(ctx, edgeData.Source, edgeData.Target); err != nil {
		return false, fmt.Errorf("failed to persist edge: %v", err)
	}

	return true, nil
}

func (gm *GraphManager) handleRemoveEdge(ctx context.Context, cmd etcd.Command) (bool, error) {
	var edgeData struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.Unmarshal([]byte(cmd.Data.(string)), &edgeData); err != nil {
		return false, fmt.Errorf("failed to unmarshal edge data: %v", err)
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.graph.RemoveEdge(dag.BasicEdge(edgeData.Source, edgeData.Target))

	// Remove from etcd
	if err := gm.removeEdgeFromEtcd(ctx, edgeData.Source, edgeData.Target); err != nil {
		return false, fmt.Errorf("failed to remove edge from etcd: %v", err)
	}

	return true, nil
}

func (gm *GraphManager) handleHeartbeat(ctx context.Context, cmd etcd.Command) (bool, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.nodeLastSeen[cmd.NodeID] = time.Now()
	return true, nil
}

func (gm *GraphManager) cleanupStaleNodes() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	now := time.Now()
	for nodeID, lastSeen := range gm.nodeLastSeen {
		if now.Sub(lastSeen) > 5*time.Minute {
			gm.graph.Remove(nodeID)
			delete(gm.nodeLastSeen, nodeID)
			// Remove from etcd asynchronously
			go func(id string) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := gm.removeNodeFromEtcd(ctx, id); err != nil {
					fmt.Printf("Failed to remove stale node %s from etcd: %v\n", id, err)
				}
			}(nodeID)
		}
	}
}

func (gm *GraphManager) sendHeartbeat(ctx context.Context) error {
	heartbeat := etcd.Command{
		ID:     fmt.Sprintf("heartbeat-%s-%d", gm.macAddress, gm.processID),
		Type:   "graph_heartbeat",
		NodeID: gm.macAddress,
		Data:   fmt.Sprintf("%d", gm.processID),
	}

	_, err := gm.etcdManager.SendCommand(heartbeat)
	return err
}

func (gm *GraphManager) persistNode(ctx context.Context, nodeID string) error {
	key := nodePrefix + nodeID
	value, err := json.Marshal(map[string]string{"id": nodeID})
	if err != nil {
		return err
	}
	return gm.etcdManager.Put(ctx, key, string(value))
}

func (gm *GraphManager) removeNodeFromEtcd(ctx context.Context, nodeID string) error {
	key := nodePrefix + nodeID
	return gm.etcdManager.Delete(ctx, key)
}

func (gm *GraphManager) persistEdge(ctx context.Context, source, target string) error {
	key := fmt.Sprintf("%s%s-%s", edgePrefix, source, target)
	value, err := json.Marshal(map[string]string{"source": source, "target": target})
	if err != nil {
		return err
	}
	return gm.etcdManager.Put(ctx, key, string(value))
}

func (gm *GraphManager) removeEdgeFromEtcd(ctx context.Context, source, target string) error {
	key := fmt.Sprintf("%s%s-%s", edgePrefix, source, target)
	return gm.etcdManager.Delete(ctx, key)
}

func (gm *GraphManager) syncGraphFromEtcd(ctx context.Context) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Sync nodes
	nodes, err := gm.etcdManager.Get(ctx, nodePrefix, etcd.OperationOptions{Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("failed to get nodes from etcd: %v", err)
	}
	var nodeData struct {
		ID string `json:"id"`
	}
	for _, node := range strings.Split(nodes, "\n") {
		if node == "" {
			continue
		}
		if err := json.Unmarshal([]byte(node), &nodeData); err != nil {
			return fmt.Errorf("failed to unmarshal node data: %v", err)
		}
		gm.graph.Add(nodeData.ID)
		gm.nodeLastSeen[nodeData.ID] = time.Now()
	}

	// Sync edges
	edges, err := gm.etcdManager.Get(ctx, edgePrefix, etcd.OperationOptions{Timeout: 5 * time.Second})
	if err != nil {
		return fmt.Errorf("failed to get edges from etcd: %v", err)
	}
	var edgeData struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	for _, edge := range strings.Split(edges, "\n") {
		if edge == "" {
			continue
		}
		if err := json.Unmarshal([]byte(edge), &edgeData); err != nil {
			return fmt.Errorf("failed to unmarshal edge data: %v", err)
		}
		gm.graph.Connect(dag.BasicEdge(edgeData.Source, edgeData.Target))
	}

	return nil
}

func (gm *GraphManager) watchGraphChanges(ctx context.Context) {
	watchChan := gm.etcdManager.Watch(ctx, graphPrefix)
	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			gm.handleEtcdEvent(ctx, event)
		}
	}
}

func (gm *GraphManager) handleEtcdEvent(ctx context.Context, event *clientv3.Event) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	key := string(event.Kv.Key)
	switch {
	case strings.HasPrefix(key, nodePrefix):
		nodeID := strings.TrimPrefix(key, nodePrefix)
		if event.Type == clientv3.EventTypePut {
			gm.graph.Add(nodeID)
			gm.nodeLastSeen[nodeID] = time.Now()
		} else if event.Type == clientv3.EventTypeDelete {
			gm.graph.Remove(nodeID)
			delete(gm.nodeLastSeen, nodeID)
		}
	case strings.HasPrefix(key, edgePrefix):
		edgeParts := strings.TrimPrefix(key, edgePrefix)
		parts := strings.Split(edgeParts, "-")
		if len(parts) != 2 {
			fmt.Printf("Invalid edge key: %s\n", key)
			return
		}
		source, target := parts[0], parts[1]
		if event.Type == clientv3.EventTypePut {
			gm.graph.Connect(dag.BasicEdge(source, target))
		} else if event.Type == clientv3.EventTypeDelete {
			gm.graph.RemoveEdge(dag.BasicEdge(source, target))
		}
	}
}

func getMacAddress() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, err := iface.Addrs()
			if err != nil {
				return "", err
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return iface.HardwareAddr.String(), nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no valid MAC address found")
}
