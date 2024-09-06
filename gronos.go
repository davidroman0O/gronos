package gronos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/avast/retry-go/v3"
	"github.com/davidroman0O/gronos/etcd"
	"github.com/davidroman0O/gronos/graph"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Primitive interface {
	int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64 | string | bool
}

type Gronos struct {
	EtcdManager     *etcd.EtcdManager
	CommandHandlers []CommandHandler
	options         Options
}

type CommandHandler interface {
	HandleCommand(ctx context.Context, cmd etcd.Command) (bool, error)
	Run(ctx context.Context, etcd *etcd.EtcdManager) error
}

func New[T Primitive](opts Options) (*Gronos, error) {
	g := &Gronos{
		CommandHandlers: []CommandHandler{},
	}

	etcdOpts := []etcd.Option{
		etcd.WithMode(opts.Mode),
		etcd.WithDataDir(opts.DataDir),
	}

	if opts.RemoveDataDir {
		etcdOpts = append(etcdOpts, etcd.WithRemoveDataDir())
	}

	// Port Configuration:
	// - Client Port: Used by all clients (including followers and beacons) to interact with the etcd cluster.
	// - Peer Port: Used for inter-node communication within the etcd cluster.
	//
	// Followers and beacons use the client port to communicate with the etcd cluster.
	// The peer port is used internally by etcd nodes for cluster state management.
	//
	// Mode-specific Requirements:
	//
	// 1. Standalone:
	//    - Requires both peer and client ports
	//    - Combines etcd embed, leader, beacon, and follower(s) in one runtime
	//    - Needs entry ports for etcd peer and client, etcd endpoints, and beacon address
	//
	// 2. LeaderEmbed:
	//    - Requires both peer and client ports
	//    - Combines etcd embed and leader
	//    - Needs entry ports for etcd peer and client
	//    - TODO: Determine if we can know our own endpoint
	//
	// 3. LeaderRemote:
	//    - Requires client port and endpoints
	//    - Leader runtime only
	//    - Needs entry ports for peer and client, and etcd endpoint
	//
	// 4. Follower:
	//    - Requires client port and endpoints
	//    - Follower runtime only
	//    - Needs etcd endpoint and beacon address
	//
	// 5. Beacon:
	//    - Requires client port and endpoints
	//    - Beacon runtime only
	//    - Needs etcd endpoint
	switch opts.Mode {

	case etcd.ModeStandalone, etcd.ModeLeaderEmbed:
		etcdOpts = append(etcdOpts, etcd.WithPorts(opts.PeerPort, opts.ClientPort))

	case etcd.ModeLeaderRemote, etcd.ModeFollower:
		if len(opts.Endpoints) > 0 {
			etcdOpts = append(etcdOpts, etcd.WithEndpoints(opts.Endpoints))
		}
		if opts.BeaconAddr == "" {
			return nil, fmt.Errorf("beacon address is required for follower mode")
		}
		var etcdEndpoints []string
		// retry
		if err := retry.Do(
			func() error {
				var err error
				fmt.Println("Getting etcd endpoint from beacon...")
				etcdEndpoints, err = getEtcdEndpointFromBeacon(opts.BeaconAddr)
				if err != nil {
					return fmt.Errorf("failed to get etcd endpoint from beacon: %v", err)
				}
				return nil
			},
			retry.Attempts(10),
			retry.Delay(3*time.Second),
		); err != nil {
			return nil, fmt.Errorf("failed to get etcd endpoint from beacon: %v", err)
		}
		fmt.Println("etcd endpoint:", etcdEndpoints)
		etcdOpts = append(etcdOpts, etcd.WithEndpoints(etcdEndpoints))

	case etcd.ModeBeacon:
		etcdOpts = append(etcdOpts, etcd.WithBeacon(opts.BeaconAddr))
		etcdOpts = append(etcdOpts, etcd.WithEndpoints(opts.Endpoints))

	}

	g.options = opts

	var err error
	g.EtcdManager, err = etcd.NewEtcd(etcdOpts...)
	if err != nil {
		return nil, err
	}

	// Beacon mode doesn't need graph manager
	if opts.Mode != etcd.ModeBeacon {
		// Add graph manager
		gm, err := graph.NewGraphManager()
		if err != nil {
			return nil, err
		}
		g.CommandHandlers = append(g.CommandHandlers, gm) // the run will start soon
	}

	if opts.Mode != etcd.ModeLeaderEmbed && opts.Mode != etcd.ModeLeaderRemote {
		fmt.Println("Adding health check")
		g.CommandHandlers = append(g.CommandHandlers, NewHealthCheck(opts.Mode.String()))
	}

	return g, nil
}

func getEtcdEndpointFromBeacon(beaconAddr string) ([]string, error) {
	conn, err := net.DialTimeout("tcp", beaconAddr, time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to beacon: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(1 * time.Second))

	data, err := io.ReadAll(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read etcd endpoint from beacon: %v", err)
	}

	var endpoints []string
	if err := json.Unmarshal(data, &endpoints); err != nil {
		return nil, fmt.Errorf("failed to deserialize etcd endpoints: %v", err)
	}

	return endpoints, nil
}

func (g *Gronos) Run(ctx context.Context) error {
	fmt.Printf("Gronos started in %s mode\n", g.options.Mode)

	for _, handler := range g.CommandHandlers {
		go func(h CommandHandler) {
			if err := h.Run(ctx, g.EtcdManager); err != nil {
				fmt.Printf("Error in command handler: %v\n", err)
			}
		}(handler)
	}

	if g.options.Mode == etcd.ModeBeacon {
		fmt.Printf("Beacon listening on %s\n", g.options.BeaconAddr)
		fmt.Printf("Serving etcd endpoint: %v\n", g.options.Endpoints)

		if err := g.EtcdManager.Ping(); err != nil {
			return fmt.Errorf("failed to ping etcd: %v", err)
		}

		fmt.Println("Beacon pinged etcd successfully")

		now := time.Now()
		fmt.Println("Beacon sending:", now.Format(time.RFC3339))
		if err := g.EtcdManager.Put(context.Background(), "beacon", now.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("failed to put beacon key: %v", err)
		}

		vany, err := g.EtcdManager.Get(context.Background(), "beacon")
		if err != nil {
			return fmt.Errorf("failed to get beacon key: %v", err)
		}
		fmt.Println("Beacon value:", vany)
		<-ctx.Done()
		return ctx.Err()
	}

	watchChan := g.EtcdManager.WatchCommands(ctx)

	defer g.EtcdManager.Close()

	if g.options.Mode == etcd.ModeFollower {
		fmt.Println("Follower waiting for commands...")
		<-ctx.Done()
		return ctx.Err()
	}

	fmt.Println("Leader processing commands...")

	// Only the leader will process the commands
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case watchResp := <-watchChan:
			for _, ev := range watchResp.Events {
				fmt.Println("Received event:", ev)
				if ev.Type == clientv3.EventTypePut {
					var cmd etcd.Command
					if err := json.Unmarshal(ev.Kv.Value, &cmd); err != nil {
						// Handle error
						continue
					}
					for _, handler := range g.CommandHandlers {
						processed, err := handler.HandleCommand(ctx, cmd)
						if err != nil {
							// Handle error
							fmt.Println(err)
						}
						if processed {
							break
						}
					}
				}
			}
		}
	}
}
