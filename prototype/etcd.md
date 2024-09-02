I'm going to re-write everthing 

```go

type LifecycleFunction interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

```

I want to let etcd handle the state data, we will just maintain the graph locally and will fetch from etcd.

```go

type EtcdMode int

const (
	FullStandalone EtcdMode = iota
	EmbeddedServer
	RemoteServer
)

func NewEtcdClient(ctx context.Context, mode EtcdMode, endpoints []string) (*clientv3.Client, error) {
	switch mode {
	case FullStandalone:
		cfg := embed.NewConfig()
		e, err := embed.StartEtcd(cfg)
		if err != nil {
			return nil, err
		}
		<-e.Server.ReadyNotify()
		return e.Client(), nil
	case EmbeddedServer:
		cfg := embed.NewConfig()
		cfg.LCUrls = append(cfg.LCUrls, endpoints...)
		e, err := embed.StartEtcd(cfg)
		if err != nil {
			return nil, err
		}
		<-e.Server.ReadyNotify()
		return e.Client(), nil
	case RemoteServer:
		return clientv3.New(clientv3.Config{
			Endpoints: endpoints,
		})
	default:
		return nil, nil
	}
}

```

We need to keep the same context isolation, one LF == one context.

Each action catched by the watchers will be executed one by one.

The leader node will have a watcher of events that will process those events/actions, that way it will works even if we split it.

```go
package types

type Action struct {
    Type   string `json:"type"`
    NodeID string `json:"nodeId"`
    Data   string `json:"data"`
}

type DAGNode struct {
    ID       string   `json:"id"`
    Data     string   `json:"data"`
    Children []string `json:"children"`
}

type DAG struct {
    Nodes map[string]*DAGNode `json:"nodes"`
}
```

