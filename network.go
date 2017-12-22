package testingdock

import (
	"context"
	"testing"

	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// NetworkOpts is used when creating a new network.
type NetworkOpts struct {
	Name string
}

// Network is a struct representing a docker network configuration.
// This should usually not be created directly but via the NewNetwork
// function or in the Suite.
type Network struct {
	t        testing.TB
	cli      *client.Client // docker API object to talk to the docker daemon
	id, name string
	gateway  string
	cancel   func()
	children []*Container
	closed   bool
}

// NewNetwork creates a new docker network configuration with the given options.
func NewNetwork(t testing.TB, c *client.Client, opts NetworkOpts) *Network {
	return &Network{
		t:    t,
		cli:  c,
		name: opts.Name,
	}
}

// Start creates the actual docker network.
func (n *Network) Start(ctx context.Context) {
	n.initialCleanup(ctx)

	res, err := n.cli.NetworkCreate(ctx, n.name, types.NetworkCreate{})
	if err != nil {
		n.t.Fatalf("network creation failure: %s", err.Error())
	}
	n.id = res.ID
	n.cancel = func() {
		if n.closed {
			return
		}
		if err := n.cli.NetworkRemove(ctx, n.id); err != nil {
			n.t.Fatalf("network removal failure: %s", err.Error())
		}
		printf("(cancel) %-25s (%s) - network removed", n.name, n.id)
	}
	printf("(setup ) %-25s (%s) - network created", n.name, n.id)

	ni, err := n.cli.NetworkInspect(ctx, n.id, false)
	if err != nil {
		n.cancel()
		n.t.Fatalf("network inspect failure: %s", err.Error())
	}
	n.gateway = ni.IPAM.Config[0].Gateway
	printf("(setup ) %-25s (%s) - network got gateway ip: %s", n.name, n.id, n.gateway)
	for _, cont := range n.children {
		cont.Start(ctx)
	}
}

// removes the network if it already exists and all containers being part
// of that network
func (n *Network) initialCleanup(ctx context.Context) {
	networkListArgs := filters.NewArgs()
	networkListArgs.Add("name", n.name)

	networks, err := n.cli.NetworkList(ctx, types.NetworkListOptions{Filters: networkListArgs})
	if err != nil {
		n.t.Fatalf("network listing failure: %s", err.Error())
	}
	for _, nn := range networks {
		containers, err := n.cli.ContainerList(ctx, types.ContainerListOptions{All: true})
		if err != nil {
			n.t.Fatalf("container list failure: %s", err.Error())
		}
		for _, cc := range containers {
			for _, nnn := range cc.NetworkSettings.Networks {
				if nnn.NetworkID == nn.ID {
					if err = n.cli.ContainerRemove(ctx, cc.ID, types.ContainerRemoveOptions{
						RemoveVolumes: true,
						Force:         true,
					}); err != nil {
						n.t.Fatalf("container removal failure: %s", err.Error())
					}
					printf("(setup ) %-25s (%s) - network endpoint removed: %s", nn.Name, nn.ID, cc.Names[0])
				}
			}
		}
		if err = n.cli.NetworkRemove(ctx, nn.ID); err != nil {
			n.t.Fatalf("network removal failure: %s", err.Error())
		}
		printf("(setup ) %-25s (%s) - network removed", nn.Name, nn.ID)
	}
}

// Close the docker network. This also closes the
// children containers if any are set in the Network struct.
// Implements io.Closer interface.
func (n *Network) Close() error {
	for _, cont := range n.children {
		cont.Close()
	}
	n.cancel()
	n.closed = true
	return nil
}

// After adds a child container to the current network configuration.
// These containers then kind of "depend" on the network and will
// be closed when the network closes.
func (n *Network) After(c *Container) {
	c.network = n
	n.children = append(n.children, c)
}

// resets the network and the child containers.
func (n *Network) reset(ctx context.Context) {
	now := time.Now()
	for _, c := range n.children {
		c.Reset(ctx)
	}
	printf("(reset ) %-25s (%s) - network reseted in %s", n.name, n.id, time.Since(now))
}
