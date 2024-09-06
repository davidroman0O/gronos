package gronos

import (
	"context"
	"fmt"
	"time"

	"github.com/davidroman0O/gronos/etcd"
)

type HealthCheck struct {
	name string
}

func NewHealthCheck(name string) *HealthCheck {
	return &HealthCheck{
		name: name,
	}
}

func (h *HealthCheck) HandleCommand(ctx context.Context, cmd etcd.Command) (bool, error) {
	fmt.Println("Handling command", cmd)
	return true, nil
}

func (h *HealthCheck) Run(ctx context.Context, manager *etcd.EtcdManager) error {
	fmt.Println("HealthCheck running...")
	// timer every seconds to ping the etcd
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("HealthCheck shutting down...")
			return ctx.Err()
		case <-ticker.C:
			heartbeat := etcd.Command{
				ID:   fmt.Sprintf("heartbeat-%v", h.name),
				Type: "health_check",
			}
			_, err := manager.SendCommand(heartbeat)
			fmt.Println("HealthCheck still active...", err)
			if err == nil {
				if err = manager.Delete(context.Background(), heartbeat.ID); err != nil {
					fmt.Println("Error deleting command", err)
				}
				fmt.Println("HealthCheck key deleted successfully")
			}
		}
	}
}
