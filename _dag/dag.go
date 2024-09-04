package dag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/davidroman0O/gronos/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Node struct {
	ID       string                 `json:"id"`
	Data     map[string]interface{} `json:"data"`
	Children []string               `json:"children"`
}

type DAG struct {
	nodes map[string]*Node
	etcd  *etcd.EtcdManager
	mu    sync.RWMutex
}

func NewDAG(etcdManager *etcd.EtcdManager) *DAG {
	dag := &DAG{
		nodes: make(map[string]*Node),
		etcd:  etcdManager,
	}
	dag.loadFromEtcd()
	go dag.watchCommands()
	return dag
}

func (d *DAG) AddNode(id string, data map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.nodes[id]; exists {
		return fmt.Errorf("node with ID %s already exists", id)
	}

	d.nodes[id] = &Node{
		ID:       id,
		Data:     data,
		Children: []string{},
	}

	return d.saveToEtcd()
}

func (d *DAG) AddEdge(parentID, childID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	parent, parentExists := d.nodes[parentID]
	_, childExists := d.nodes[childID]

	if !parentExists || !childExists {
		return fmt.Errorf("parent or child node does not exist")
	}

	for _, existingChild := range parent.Children {
		if existingChild == childID {
			return fmt.Errorf("edge already exists")
		}
	}

	parent.Children = append(parent.Children, childID)
	return d.saveToEtcd()
}

func (d *DAG) UpdateNode(id string, data map[string]interface{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.nodes[id]
	if !exists {
		return fmt.Errorf("node with ID %s does not exist", id)
	}

	node.Data = data
	return d.saveToEtcd()
}

func (d *DAG) DeleteNode(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.nodes[id]; !exists {
		return fmt.Errorf("node with ID %s does not exist", id)
	}

	delete(d.nodes, id)

	for _, node := range d.nodes {
		for i, childID := range node.Children {
			if childID == id {
				node.Children = append(node.Children[:i], node.Children[i+1:]...)
				break
			}
		}
	}

	return d.saveToEtcd()
}

func (d *DAG) saveToEtcd() error {
	dagJSON, err := json.Marshal(d.nodes)
	if err != nil {
		return fmt.Errorf("failed to marshal DAG: %v", err)
	}

	return d.etcd.Put("dag", string(dagJSON))
}

func (d *DAG) loadFromEtcd() error {
	dagJSON, err := d.etcd.Get("dag")
	if err != nil {
		return fmt.Errorf("failed to get DAG from etcd: %v", err)
	}

	if dagJSON != "" {
		err = json.Unmarshal([]byte(dagJSON), &d.nodes)
		if err != nil {
			return fmt.Errorf("failed to unmarshal DAG: %v", err)
		}
	}

	return nil
}

func (d *DAG) ProcessCommand(cmd etcd.Command) etcd.Response {
	var err error
	switch cmd.Type {
	case "add":
		data, ok := cmd.Data.(map[string]interface{})
		if !ok {
			return etcd.Response{CommandID: cmd.ID, Success: false, Message: "Invalid data format for add command"}
		}
		err = d.AddNode(cmd.NodeID, data)
	case "update":
		data, ok := cmd.Data.(map[string]interface{})
		if !ok {
			return etcd.Response{CommandID: cmd.ID, Success: false, Message: "Invalid data format for update command"}
		}
		err = d.UpdateNode(cmd.NodeID, data)
	case "delete":
		err = d.DeleteNode(cmd.NodeID)
	case "addEdge":
		childID, ok := cmd.Data.(string)
		if !ok {
			return etcd.Response{CommandID: cmd.ID, Success: false, Message: "Invalid data format for addEdge command"}
		}
		err = d.AddEdge(cmd.NodeID, childID)
	default:
		return etcd.Response{CommandID: cmd.ID, Success: false, Message: fmt.Sprintf("Unknown command type: %s", cmd.Type)}
	}

	if err != nil {
		return etcd.Response{CommandID: cmd.ID, Success: false, Message: err.Error()}
	}
	return etcd.Response{CommandID: cmd.ID, Success: true, Message: "Command processed successfully"}
}

func (d *DAG) watchCommands() {
	watchChan := d.etcd.WatchCommands(context.Background())
	for watchResp := range watchChan {
		for _, ev := range watchResp.Events {
			if ev.Type == clientv3.EventTypePut {
				var cmd etcd.Command
				err := json.Unmarshal(ev.Kv.Value, &cmd)
				if err != nil {
					log.Printf("Error unmarshaling command: %v\n", err)
					continue
				}
				resp := d.ProcessCommand(cmd)
				err = d.etcd.SendResponse(resp)
				if err != nil {
					log.Printf("Error sending response: %v\n", err)
				}
			}
		}
	}
}
