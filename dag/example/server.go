package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/davidroman0O/gronos/dag"
	"github.com/davidroman0O/gronos/etcd"
)

func main() {
	etcdManager, err := etcd.NewEtcdManager(
		etcd.WithMode(etcd.ModeServerEmbed),
		etcd.WithDataDir("./data/server_data"),
		etcd.WithPorts(2380, 2379),
		etcd.WithUniqueDataDir(),
	)
	if err != nil {
		log.Fatalf("Failed to create etcd manager: %v", err)
	}
	defer etcdManager.Close()

	dagInstance := dag.NewDAG(etcdManager)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching for commands
	go watchCommands(ctx, etcdManager, dagInstance)

	fmt.Println("DAG Server is running. Press CTRL+C to exit.")

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("Shutting down server...")
	cancel() // Cancel the context to stop the command watcher
}

func watchCommands(ctx context.Context, em *etcd.EtcdManager, d *dag.DAG) {
	watchChan := em.WatchCommands(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case watchResp := <-watchChan:
			for _, ev := range watchResp.Events {
				var cmd etcd.Command
				err := json.Unmarshal(ev.Kv.Value, &cmd)
				if err != nil {
					log.Printf("Error unmarshaling command: %v\n", err)
					continue
				}
				response := d.ProcessCommand(cmd)
				err = em.SendResponse(response)
				if err != nil {
					log.Printf("Error sending response: %v\n", err)
				}
			}
		}
	}
}
