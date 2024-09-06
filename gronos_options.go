package gronos

import (
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/davidroman0O/gronos/etcd"
)

// type OptionsFollower struct{}
// type OptionsLeader struct{}
// type OptionsBeacon struct{}

type Options struct {
	Name          string
	Mode          etcd.Mode
	DataDir       string
	PeerPort      int
	ClientPort    int
	Endpoints     []string
	BeaconAddr    string
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
			ClientPort:    2379, // Default client port
		},
	}
	builder.setDefaultsForMode()
	return builder
}

func (b *OptionsBuilder) setDefaultsForMode() {
	b.opts.DataDir = filepath.Join(os.TempDir(), fmt.Sprintf("gronos-%s", b.opts.Mode))
	switch b.opts.Mode {
	case etcd.ModeStandalone, etcd.ModeLeaderEmbed:
		b.opts.PeerPort = findAvailablePort(2380) // Start looking from default peer port
	case etcd.ModeLeaderRemote, etcd.ModeFollower:
		b.opts.Endpoints = []string{"localhost:2379"}
	case etcd.ModeBeacon:
		b.opts.BeaconAddr = "localhost:5000"
		b.opts.Endpoints = []string{"localhost:2379"}
	}
	// I don't think we need to search for this port to be available
	// by default everyone expect to connect to it
	// b.opts.ClientPort = findAvailablePort(b.opts.ClientPort)
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

func (b *OptionsBuilder) WithRemoveDataDir() *OptionsBuilder {
	b.opts.RemoveDataDir = true
	return b
}

func (b *OptionsBuilder) WithName(name string) *OptionsBuilder {
	b.opts.Name = name
	return b
}

func (b *OptionsBuilder) Build() (Options, error) {
	if err := b.validate(); err != nil {
		return Options{}, err
	}

	b.applyEnvironmentOverrides()

	// // Ensure ports are available
	// b.opts.ClientPort = findAvailablePort(b.opts.ClientPort)
	// if b.opts.Mode == etcd.ModeStandalone || b.opts.Mode == etcd.ModeLeaderEmbed {
	// 	b.opts.PeerPort = findAvailablePort(b.opts.PeerPort)
	// }

	// // Log port assignments
	// log.Printf("Assigned client port: %d", b.opts.ClientPort)
	// if b.opts.Mode == etcd.ModeStandalone || b.opts.Mode == etcd.ModeLeaderEmbed {
	// 	log.Printf("Assigned peer port: %d", b.opts.PeerPort)
	// }

	// Create data directory if it doesn't exist
	// if b.opts.Mode == etcd.ModeBeacon || b.opts.Mode == etcd.ModeStandalone || b.opts.Mode == etcd.ModeLeaderEmbed {
	if b.opts.DataDir != "" {
		if err := os.MkdirAll(b.opts.DataDir, 0755); err != nil {
			return Options{}, fmt.Errorf("failed to create data directory: %v", err)
		}
	}
	// }

	if b.opts.Name == "" {
		b.opts.Name = fmt.Sprintf("gronos-%s", b.opts.Mode)
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
		if len(b.opts.Endpoints) == 0 {
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
}

func findAvailablePort(startPort int) int {
	for port := startPort; port < 65535; port++ {
		if isPortAvailable(port) {
			return port
		}
	}
	log.Fatalf("No available ports found")
	return 0
}

func isPortAvailable(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
