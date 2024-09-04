package main

import (
	"fmt"
	"log"
	"time"

	"github.com/davidroman0O/gronos/etcd"
	"github.com/google/uuid"
)

func main() {
	etcdManager, err := etcd.NewEtcdManager(
		etcd.WithMode(etcd.ModeClientEmbed),
		etcd.WithDataDir("./data/client_data"),
		etcd.WithPorts(2382, 2381),
		etcd.WithUniqueDataDir(),
	)
	if err != nil {
		log.Fatalf("Failed to create etcd manager: %v", err)
	}
	defer etcdManager.Close()

	commands := []etcd.Command{
		{ID: uuid.New().String(), Type: "add", NodeID: "1", Data: map[string]interface{}{"name": "Node 1", "value": 10}},
		{ID: uuid.New().String(), Type: "add", NodeID: "2", Data: map[string]interface{}{"name": "Node 2", "value": 20}},
		{ID: uuid.New().String(), Type: "addEdge", NodeID: "1", Data: "2"},
		{ID: uuid.New().String(), Type: "update", NodeID: "2", Data: map[string]interface{}{"name": "Updated Node 2", "value": 25}},
		{ID: uuid.New().String(), Type: "delete", NodeID: "1"},
	}

	for _, cmd := range commands {
		fmt.Printf("Sending command: %+v\n", cmd)
		respChan, err := etcdManager.SendCommand(cmd)
		if err != nil {
			log.Printf("Error sending command: %v\n", err)
			continue
		}

		select {
		case resp := <-respChan:
			fmt.Printf("Received response: %+v\n", resp)
		case <-time.After(1 * time.Second):
			fmt.Printf("Timeout waiting for response to command %s\n", cmd.ID)
		}
	}
}
