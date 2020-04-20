package testingdock_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	_ "github.com/lib/pq"

	"github.com/dapperlabs/testingdock"
)

func TestContainer_Start(t *testing.T) {
	// set up docker client
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		t.Fatalf("error creating docker client: %s", err.Error())
	}

	// create suite
	name := "TestContainer_Start"
	s, ok := testingdock.GetOrCreateSuite(t, name, testingdock.SuiteOpts{
		Client: cli,
	})
	if ok {
		t.Fatal("this suite should not exists yet")
	}

	// create network
	n := s.Network(testingdock.NetworkOpts{
		Name: name,
	})

	// create postgres and mnemosyne configurations
	postgresPort := testingdock.RandomPort(t)
	mnemosynePort := testingdock.RandomPort(t)
	mnemosyneDebugPort := testingdock.RandomPort(t)

	db, err := sql.Open("postgres", "postgres://postgres:@localhost:"+postgresPort+"?sslmode=disable")
	if err != nil {
		t.Fatalf("database connection error: %s", err.Error())
	}
	postgres := s.Container(testingdock.ContainerOpts{
		Name:      "postgres",
		ForcePull: false,
		Config: &container.Config{
			Image: "postgres:9.6",
			Env:   []string{"POSTGRES_HOST_AUTH_METHOD=trust"},
		},
		HostConfig: &container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port("5432/tcp"): []nat.PortBinding{
					{
						HostPort: postgresPort,
					},
				},
			},
		},
		HealthCheck: testingdock.HealthCheckCustom(db.Ping),
		Reset: testingdock.ResetCustom(func() error {
			_, err := db.Exec(`
				DROP SCHEMA public CASCADE;
				DROP SCHEMA mnemosyne CASCADE;
				CREATE SCHEMA public;
			`)
			return err
		}),
	})
	mnemosyned := s.Container(testingdock.ContainerOpts{
		Name:      "mnemosyned",
		ForcePull: true,
		Config: &container.Config{
			Image: "piotrkowalczuk/mnemosyne:v0.8.4",
		},
		HostConfig: &container.HostConfig{
			PortBindings: nat.PortMap{
				nat.Port("8080/tcp"): []nat.PortBinding{{HostPort: mnemosynePort}},
				nat.Port("8081/tcp"): []nat.PortBinding{{HostPort: mnemosyneDebugPort}},
			},
		},
		HealthCheck: testingdock.HealthCheckHTTP("http://localhost:" + mnemosyneDebugPort + "/health"),
	})

	randomPostgres := s.Container(testingdock.ContainerOpts{
		Name:      "randomPostgres",
		ForcePull: true,
		Config: &container.Config{
			Image: "postgres:9.6",
			Env:   []string{"POSTGRES_HOST_AUTH_METHOD=trust"},
		},
	})

	// add postgres to the test network
	n.After(postgres)
	// add another postgres to the test network
	n.After(randomPostgres)
	// start mnemosyned after postgres, this also adds it to the test network
	postgres.After(mnemosyned)

	// start the network, this also starts the containers
	s.Start(context.TODO())
	defer func() {
		s.Close()
		s.Remove()
	}()

	// test stuff within the database
	testQueries(t, db)

	s.Reset(context.TODO())

	testQueries(t, db)

	if err = s.Close(); err != nil {
		t.Fatalf("could not close containers: %s", err.Error())
	}

	list0, err := cli.ContainerList(context.TODO(), types.ContainerListOptions{All: true})
	if err != nil {
		t.Fatalf("could not retreive container list: %s", err.Error())
	}

	if err = s.Remove(); err != nil {
		t.Fatalf("could not remove containers: %s", err.Error())
	}

	list1, err := cli.ContainerList(context.TODO(), types.ContainerListOptions{All: true})
	if err != nil {
		t.Fatalf("could not retreive container list: %s", err.Error())
	}

	if len(list0) != len(list1)+3 {
		t.Fatalf("expected Remove to remove 3 containers from container list (len before %v, len after %v)", len(list0),
			len(list1))
	}
}

func testQueries(t *testing.T, db *sql.DB) {
	_, err := db.ExecContext(context.TODO(), "CREATE TABLE public.example (name TEXT);")
	if err != nil {
		t.Fatalf("table creation error: %s", err.Error())
	}
	_, err = db.ExecContext(context.TODO(), "INSERT INTO public.example (name) VALUES ('anything')")
	if err != nil {
		t.Fatalf("insert error: %s", err.Error())
	}
	_, err = db.ExecContext(context.TODO(), "INSERT INTO mnemosyne.session (access_token, refresh_token,subject_id, bag) VALUES ('123', '123', '1', '{}')")
	if err != nil {
		t.Fatalf("insert error: %s", err.Error())
	}
}
