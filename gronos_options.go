package gronos

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/davidroman0O/gronos/etcd"
)

type Options struct {
	Mode          etcd.Mode
	DataDir       string
	PeerPort      int
	ClientPort    int
	Endpoints     []string
	BeaconAddr    string
	EtcdEndpoint  string
	RemoveDataDir bool
}

type OptionsBuilder struct {
	opts Options
}

func NewOptionsBuilder(mode etcd.Mode) *OptionsBuilder {
	builder := &OptionsBuilder{
		opts: Options{
			Mode:          mode,
			RemoveDataDir: false,
		},
	}
	builder.setDefaultsForMode()
	return builder
}

func (b *OptionsBuilder) setDefaultsForMode() {
	b.opts.DataDir = filepath.Join(os.TempDir(), fmt.Sprintf("gronos-%s", b.opts.Mode))
	switch b.opts.Mode {
	case etcd.ModeStandalone, etcd.ModeLeaderEmbed:
		b.opts.PeerPort = 2380
		b.opts.ClientPort = 2379
	case etcd.ModeLeaderRemote, etcd.ModeFollower:
		b.opts.Endpoints = []string{"localhost:2379"}
	case etcd.ModeBeacon:
		b.opts.BeaconAddr = "localhost:5000"
		b.opts.EtcdEndpoint = "localhost:2379"
	}
}

func (b *OptionsBuilder) WithDataDir(dir string) *OptionsBuilder {
	b.opts.DataDir = dir
	return b
}

func (b *OptionsBuilder) WithPorts(peerPort, clientPort int) *OptionsBuilder {
	b.opts.PeerPort = peerPort
	b.opts.ClientPort = clientPort
	return b
}

func (b *OptionsBuilder) WithEndpoints(endpoints []string) *OptionsBuilder {
	b.opts.Endpoints = endpoints
	return b
}

func (b *OptionsBuilder) WithBeacon(addr string) *OptionsBuilder {
	b.opts.BeaconAddr = addr
	return b
}

func (b *OptionsBuilder) WithEtcdEndpoint(endpoint string) *OptionsBuilder {
	b.opts.EtcdEndpoint = endpoint
	return b
}

func (b *OptionsBuilder) WithRemoveDataDir() *OptionsBuilder {
	b.opts.RemoveDataDir = true
	return b
}

func (b *OptionsBuilder) Build() (Options, error) {
	if err := b.validate(); err != nil {
		return Options{}, err
	}

	b.applyEnvironmentOverrides()

	// Create data directory if it doesn't exist
	if b.opts.Mode == etcd.ModeBeacon || b.opts.Mode == etcd.ModeStandalone || b.opts.Mode == etcd.ModeLeaderEmbed {
		if b.opts.DataDir != "" {
			if err := os.MkdirAll(b.opts.DataDir, 0755); err != nil {
				return Options{}, fmt.Errorf("failed to create data directory: %v", err)
			}
		}
	}

	return b.opts, nil
}

func (b *OptionsBuilder) validate() error {
	switch b.opts.Mode {
	case etcd.ModeStandalone, etcd.ModeLeaderEmbed:
		if b.opts.DataDir == "" {
			return fmt.Errorf("DataDir is required for %s mode", b.opts.Mode)
		}
		if b.opts.PeerPort == 0 || b.opts.ClientPort == 0 {
			return fmt.Errorf("Both PeerPort and ClientPort are required for %s mode", b.opts.Mode)
		}
	case etcd.ModeLeaderRemote, etcd.ModeFollower:
		if len(b.opts.Endpoints) == 0 && b.opts.BeaconAddr == "" {
			return fmt.Errorf("Either Endpoints or BeaconAddr is required for %s mode", b.opts.Mode)
		}
	case etcd.ModeBeacon:
		if b.opts.BeaconAddr == "" {
			return fmt.Errorf("BeaconAddr is required for Beacon mode")
		}
		if b.opts.EtcdEndpoint == "" {
			return fmt.Errorf("EtcdEndpoint is required for Beacon mode")
		}
	default:
		return fmt.Errorf("Unsupported mode: %v", b.opts.Mode)
	}
	return nil
}

func (b *OptionsBuilder) applyEnvironmentOverrides() {
	if envDir := os.Getenv("GRONOS_DATA_DIR"); envDir != "" {
		b.opts.DataDir = envDir
	}
	if envPeerPort := os.Getenv("GRONOS_ETCD_PEER_PORT"); envPeerPort != "" {
		if port, err := strconv.Atoi(envPeerPort); err == nil {
			b.opts.PeerPort = port
		}
	}
	if envClientPort := os.Getenv("GRONOS_ETCD_CLIENT_PORT"); envClientPort != "" {
		if port, err := strconv.Atoi(envClientPort); err == nil {
			b.opts.ClientPort = port
		}
	}
	if envEndpoints := os.Getenv("GRONOS_ETCD_ENDPOINTS"); envEndpoints != "" && len(b.opts.Endpoints) == 0 {
		b.opts.Endpoints = []string{envEndpoints}
	}
	if envBeaconAddr := os.Getenv("GRONOS_BEACON_ADDR"); envBeaconAddr != "" {
		b.opts.BeaconAddr = envBeaconAddr
	}
	if envEtcdEndpoint := os.Getenv("GRONOS_ETCD_ENDPOINT"); envEtcdEndpoint != "" {
		b.opts.EtcdEndpoint = envEtcdEndpoint
	}
}
