package gronos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

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

	switch opts.Mode {
	case etcd.ModeStandalone, etcd.ModeLeaderEmbed:
		etcdOpts = append(etcdOpts, etcd.WithPorts(opts.PeerPort, opts.ClientPort))
	case etcd.ModeLeaderRemote, etcd.ModeFollower:
		if len(opts.Endpoints) > 0 {
			etcdOpts = append(etcdOpts, etcd.WithEndpoints(opts.Endpoints))
		} else if opts.BeaconAddr != "" {
			etcdEndpoint, err := getEtcdEndpointFromBeacon(opts.BeaconAddr)
			if err != nil {
				panic(err)
				return nil, fmt.Errorf("failed to get etcd endpoint from beacon: %v", err)
			}
			fmt.Println("etcd endpoint:", etcdEndpoint)
			etcdOpts = append(etcdOpts, etcd.WithEndpoints([]string{etcdEndpoint}))
		}
	case etcd.ModeBeacon:
		etcdOpts = append(etcdOpts, etcd.WithBeacon(opts.BeaconAddr))
		etcdOpts = append(etcdOpts, etcd.WithEndpoints([]string{opts.EtcdEndpoint}))
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

	return g, nil
}

func getEtcdEndpointFromBeacon(beaconAddr string) (string, error) {
	conn, err := net.DialTimeout("tcp", beaconAddr, 3*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to connect to beacon: %v", err)
	}
	defer conn.Close()

	endpoint, err := io.ReadAll(conn)
	if err != nil {
		return "", fmt.Errorf("failed to read etcd endpoint from beacon: %v", err)
	}

	return string(endpoint), nil
}

func (g *Gronos) Run(ctx context.Context) error {
	if g.options.Mode == etcd.ModeBeacon {
		<-ctx.Done()
		return ctx.Err()
	}

	watchChan := g.EtcdManager.WatchCommands(ctx)

	for _, handler := range g.CommandHandlers {
		go func(h CommandHandler) {
			if err := h.Run(ctx, g.EtcdManager); err != nil {
				// Handle or log error
				fmt.Println(err)
			}
		}(handler)
	}

	defer g.EtcdManager.Close()

	if g.options.Mode == etcd.ModeFollower {
		<-ctx.Done()
		return ctx.Err()
	}

	// Only the leader will process the commands
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case watchResp := <-watchChan:
			for _, ev := range watchResp.Events {
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
