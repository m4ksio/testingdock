// Package testingdock simplifies integration testing with docker.
//
// Note: this library spawns containers and networks under the label
// 'owner=testingdock', which may be subject to aggressive manipulation
// and cleanup.
package testingdock

import (
	"context"
	"testing"

	"github.com/docker/docker/client"
	"github.com/docker/docker/daemon/logger"
)

func init() {
	registry = make(map[string]*Suite)
}

var registry map[string]*Suite

// SuiteOpts is an option struct for getting or creating a suite in GetOrCreateSuite.
type SuiteOpts struct {
	// optional docker client, if one already exists
	Client *client.Client
	// whether to fail on instantiation errors
	Skip bool
	// whether to show daemon logs
	Verbose bool
}

// Suite represents a testing suite with a docker setup.
type Suite struct {
	name       string
	t          testing.TB
	cli        *client.Client
	network    *Network
	logWatcher *logger.LogWatcher
	verbose    bool
}

// GetOrCreateSuite returns a suite with the given name. If such suite is not registered yet it creates it.
// Returns true if the suite was already there, otherwise false.
func GetOrCreateSuite(t testing.TB, name string, opts SuiteOpts) (*Suite, bool) {
	if s, ok := registry[name]; ok {
		return s, true
	}

	c := opts.Client
	if c == nil {
		var err error
		c, err = client.NewEnvClient()
		if err != nil {
			if opts.Skip {
				t.Skipf("docker client instantiation failure: %s", err.Error())
			} else {
				t.Fatalf("docker client instantiation failure: %s", err.Error())
			}
		}
	}

	s := &Suite{
		cli:     c,
		t:       t,
		name:    name,
		verbose: opts.Verbose,
	}
	registry[s.name] = s
	return s, false
}

// UnregisterAll unregisters all suites by closing the networks.
func UnregisterAll() {
	printf("(unregi) start")
	for name, reg := range registry {

		if err := reg.Close(); err != nil {
			printf("(unregi) %-25s (%-64s) - suite unregister failure: %s", name, "", err.Error())
		} else {
			printf("(unregi) %-25s (%-64s) - suite unregistered", name, "")
		}
		delete(registry, name)
	}
	printf("(unregi) finished")
}

// Container creates a new docker container configuration with the given options.
func (s *Suite) Container(opts ContainerOpts) *Container {
	return newContainer(s.t, s.cli, opts)
}

// Network creates a new docker network configuration with the given options.
func (s *Suite) Network(opts NetworkOpts) *Network {
	s.network = newNetwork(s.t, s.cli, opts)
	return s.network
}

// Reset "resets" the underlying docker containers in the network. This
// calls the ResetFunc and HealthCheckFunc for each of them. These can be passed in
// ContainerOpts when creating a container.
//
// The context is passed explicitly to ResetFunc, where it can be used and
// implicitly to HealthCheckFunc where it may cancel the blocking health
// check loop.
func (s *Suite) Reset(ctx context.Context) {
	if s.network != nil {
		s.network.reset(ctx)
	}
}

// Start starts the suite. This starts all networks in the suite and the underlying containers,
// as well as the daemon logger, if Verbosity is enabled.
func (s *Suite) Start(ctx context.Context) {
	if s.logWatcher == nil && s.verbose {
		printf("(daemon) starting logging")
		s.logWatcher = logger.NewLogWatcher()
		go func() {
			for {
				select {
				case <-ctx.Done():
					printf("(daemon) stopping logging")
					s.logWatcher.Close()
					return
				case msg := <-s.logWatcher.Msg:
					printf("(daemon) %s", msg.Line)
				case err := <-s.logWatcher.Err:
					printf("(d err ) %s", err)
				}
			}
		}()
	}

	if s.network != nil {
		s.network.start(ctx)
	}
}

// Close stops the suites. This stops all networks in the suite and the underlying containers.
func (s *Suite) Close() error {
	if s.network != nil {
		return s.network.close()
	}

	return nil
}
