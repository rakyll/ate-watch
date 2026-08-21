package client

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type mockControlServer struct {
	ateapipb.UnimplementedControlServer
	actors []*ateapipb.Actor
}

func (s *mockControlServer) ListActors(ctx context.Context, in *ateapipb.ListActorsRequest) (*ateapipb.ListActorsResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	_ = md
	var results []*ateapipb.Actor
	for _, a := range s.actors {
		if in.Atespace == "" || a.GetMetadata().GetAtespace() == in.Atespace {
			results = append(results, a)
		}
	}
	return &ateapipb.ListActorsResponse{Actors: results}, nil
}

func (s *mockControlServer) GetActor(ctx context.Context, in *ateapipb.GetActorRequest) (*ateapipb.Actor, error) {
	for _, a := range s.actors {
		if a.GetMetadata().GetAtespace() == in.Actor.Atespace && a.GetMetadata().GetName() == in.Actor.Name {
			return a, nil
		}
	}
	return &ateapipb.Actor{}, nil
}

func TestClient_ListAllActors(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	srv := grpc.NewServer()
	mock := &mockControlServer{
		actors: []*ateapipb.Actor{
			{Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "act-1"}, Status: ateapipb.Actor_STATUS_RUNNING},
			{Metadata: &ateapipb.ResourceMetadata{Atespace: "team-a", Name: "act-2"}, Status: ateapipb.Actor_STATUS_SUSPENDED},
		},
	}
	ateapipb.RegisterControlServer(srv, mock)
	go srv.Serve(lis)
	defer srv.Stop()

	c, err := New(context.Background(), Options{
		Endpoint: lis.Addr().String(),
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer c.Close()

	if c.Endpoint() != lis.Addr().String() {
		t.Errorf("expected endpoint %s, got %s", lis.Addr().String(), c.Endpoint())
	}

	// List all across atespaces
	actors, err := c.ListAllActors(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAllActors failed: %v", err)
	}
	if len(actors) != 2 {
		t.Errorf("expected 2 actors, got %d", len(actors))
	}

	// List specific atespace
	teamAActors, err := c.ListAllActors(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("ListAllActors team-a failed: %v", err)
	}
	if len(teamAActors) != 1 {
		t.Errorf("expected 1 actor for team-a, got %d", len(teamAActors))
	}
}

func TestFileTokenCreds(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	if err := os.WriteFile(tokenPath, []byte("my-secret-jwt-token\n"), 0o600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}

	creds := fileTokenCreds{path: tokenPath}
	meta, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta["authorization"] != "Bearer my-secret-jwt-token" {
		t.Errorf("expected Bearer authorization header, got %q", meta["authorization"])
	}
}
