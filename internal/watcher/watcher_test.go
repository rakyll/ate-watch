package watcher

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeLister struct {
	actors []*ateapipb.Actor
	err    error
}

func (f *fakeLister) ListActors(ctx context.Context, in *ateapipb.ListActorsRequest, opts ...grpc.CallOption) (*ateapipb.ListActorsResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ateapipb.ListActorsResponse{Actors: f.actors}, nil
}

func (f *fakeLister) ReadActor(ctx context.Context, in *ateapipb.GetActorRequest, opts ...grpc.CallOption) (*ateapipb.Actor, error) {
	for _, a := range f.actors {
		if a.GetMetadata().GetAtespace() == in.Actor.Atespace && a.GetMetadata().GetName() == in.Actor.Name {
			return a, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeLister) ListAllActors(ctx context.Context, atespace string) ([]*ateapipb.Actor, error) {
	if f.err != nil {
		return nil, f.err
	}
	if atespace == "" {
		return f.actors, nil
	}
	var filtered []*ateapipb.Actor
	for _, a := range f.actors {
		if a.GetMetadata().GetAtespace() == atespace {
			filtered = append(filtered, a)
		}
	}
	return filtered, nil
}

func (f *fakeLister) Endpoint() string {
	return "fake-endpoint"
}

func (f *fakeLister) Close() error {
	return nil
}

func TestWatcher_InitialPoll(t *testing.T) {
	fake := &fakeLister{
		actors: []*ateapipb.Actor{
			{
				Metadata: &ateapipb.ResourceMetadata{
					Atespace: "default",
					Name:     "actor-1",
				},
				ActorTemplateNamespace: "ate-env",
				ActorTemplateName:      "default-env",
				Status:                 ateapipb.Actor_STATUS_RUNNING,
			},
			{
				Metadata: &ateapipb.ResourceMetadata{
					Atespace: "default",
					Name:     "actor-2",
				},
				ActorTemplateNamespace: "ate-env",
				ActorTemplateName:      "default-env",
				Status:                 ateapipb.Actor_STATUS_SUSPENDED,
			},
		},
	}

	w := New(Config{
		Lister:   fake,
		Atespace: "default",
		Interval: time.Millisecond * 100,
	})

	snap, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll failed: %v", err)
	}

	if snap.TotalCount != 2 {
		t.Errorf("expected 2 actors, got %d", snap.TotalCount)
	}
	if snap.CountByStatus[ateapipb.Actor_STATUS_RUNNING] != 1 {
		t.Errorf("expected 1 running actor, got %d", snap.CountByStatus[ateapipb.Actor_STATUS_RUNNING])
	}
	if snap.CountByStatus[ateapipb.Actor_STATUS_SUSPENDED] != 1 {
		t.Errorf("expected 1 suspended actor, got %d", snap.CountByStatus[ateapipb.Actor_STATUS_SUSPENDED])
	}
}

func TestWatcher_StatusTransition(t *testing.T) {
	actor1 := &ateapipb.Actor{
		Metadata: &ateapipb.ResourceMetadata{
			Atespace: "default",
			Name:     "actor-1",
		},
		ActorTemplateNamespace: "ate-env",
		ActorTemplateName:      "default-env",
		Status:                 ateapipb.Actor_STATUS_RUNNING,
	}

	fake := &fakeLister{actors: []*ateapipb.Actor{actor1}}
	w := New(Config{
		Lister:   fake,
		Atespace: "default",
	})

	// Initial poll
	snap1, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("first poll failed: %v", err)
	}
	if len(snap1.Actors) != 1 || snap1.Actors[0].Actor.GetStatus() != ateapipb.Actor_STATUS_RUNNING {
		t.Fatalf("expected 1 running actor")
	}

	// Status changes: RUNNING -> SUSPENDING
	actor1Updated1 := &ateapipb.Actor{
		Metadata: &ateapipb.ResourceMetadata{
			Atespace: "default",
			Name:     "actor-1",
		},
		ActorTemplateNamespace: "ate-env",
		ActorTemplateName:      "default-env",
		Status:                 ateapipb.Actor_STATUS_SUSPENDING,
	}
	fake.actors = []*ateapipb.Actor{actor1Updated1}

	snap2, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("second poll failed: %v", err)
	}
	if len(snap2.Actors) != 1 {
		t.Fatalf("expected 1 actor, got %d", len(snap2.Actors))
	}
	if snap2.Actors[0].Actor.GetStatus() != ateapipb.Actor_STATUS_SUSPENDING {
		t.Errorf("expected status SUSPENDING, got %v", snap2.Actors[0].Actor.GetStatus())
	}
	if snap2.Actors[0].PrevStatus != ateapipb.Actor_STATUS_RUNNING {
		t.Errorf("expected PrevStatus RUNNING, got %v", snap2.Actors[0].PrevStatus)
	}

	// Status changes: SUSPENDING -> SUSPENDED
	actor1Updated2 := &ateapipb.Actor{
		Metadata: &ateapipb.ResourceMetadata{
			Atespace: "default",
			Name:     "actor-1",
		},
		ActorTemplateNamespace: "ate-env",
		ActorTemplateName:      "default-env",
		Status:                 ateapipb.Actor_STATUS_SUSPENDED,
	}
	fake.actors = []*ateapipb.Actor{actor1Updated2}

	snap3, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("third poll failed: %v", err)
	}
	if len(snap3.Actors) != 1 {
		t.Fatalf("expected 1 actor, got %d", len(snap3.Actors))
	}
	if snap3.Actors[0].Actor.GetStatus() != ateapipb.Actor_STATUS_SUSPENDED {
		t.Errorf("expected status SUSPENDED, got %v", snap3.Actors[0].Actor.GetStatus())
	}
	if snap3.Actors[0].PrevStatus != ateapipb.Actor_STATUS_SUSPENDING {
		t.Errorf("expected PrevStatus SUSPENDING, got %v", snap3.Actors[0].PrevStatus)
	}
}

func TestWatcher_ActorCreatedAndDeleted(t *testing.T) {
	fake := &fakeLister{actors: []*ateapipb.Actor{}}
	w := New(Config{
		Lister:   fake,
		Atespace: "default",
	})

	// Initial poll (empty)
	snapEmpty, _ := w.Poll(context.Background())
	if snapEmpty.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", snapEmpty.TotalCount)
	}

	// Actor created
	fake.actors = []*ateapipb.Actor{
		{
			Metadata: &ateapipb.ResourceMetadata{
				Atespace: "default",
				Name:     "new-actor",
			},
			ActorTemplateName: "tmpl",
			Status:            ateapipb.Actor_STATUS_RESUMING,
		},
	}
	snap, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(snap.Actors) != 1 || snap.Actors[0].Actor.GetMetadata().GetName() != "new-actor" {
		t.Fatalf("expected created actor new-actor, got %v", snap.Actors)
	}

	// Actor deleted
	fake.actors = []*ateapipb.Actor{}
	snapDeleted, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(snapDeleted.Actors) != 0 {
		t.Fatalf("expected 0 actors after deletion, got %d", len(snapDeleted.Actors))
	}
	if snapDeleted.TotalCount != 0 {
		t.Errorf("expected total count 0, got %d", snapDeleted.TotalCount)
	}
}

func TestWatcher_StatusFilter(t *testing.T) {
	fake := &fakeLister{
		actors: []*ateapipb.Actor{
			{
				Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "act-running"},
				Status:   ateapipb.Actor_STATUS_RUNNING,
			},
			{
				Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "act-suspended"},
				Status:   ateapipb.Actor_STATUS_SUSPENDED,
			},
		},
	}
	w := New(Config{
		Lister:       fake,
		StatusFilter: []ateapipb.Actor_Status{ateapipb.Actor_STATUS_RUNNING},
	})
	snap, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(snap.Actors) != 1 {
		t.Fatalf("expected 1 filtered actor, got %d", len(snap.Actors))
	}
	if snap.Actors[0].Actor.GetMetadata().GetName() != "act-running" {
		t.Errorf("expected act-running, got %s", snap.Actors[0].Actor.GetMetadata().GetName())
	}
	if snap.TotalCount != 2 {
		t.Errorf("expected total count 2 in cluster, got %d", snap.TotalCount)
	}
}

func TestWatcher_DeterministicSorting(t *testing.T) {
	now := timestamppb.Now()
	fake := &fakeLister{
		actors: []*ateapipb.Actor{
			{
				Metadata:               &ateapipb.ResourceMetadata{Atespace: "team-b", Name: "z-actor", CreateTime: now},
				ActorTemplateNamespace: "default",
				ActorTemplateName:      "tmpl",
			},
			{
				Metadata:               &ateapipb.ResourceMetadata{Atespace: "team-a", Name: "b-actor", CreateTime: now},
				ActorTemplateNamespace: "default",
				ActorTemplateName:      "tmpl",
			},
			{
				Metadata:               &ateapipb.ResourceMetadata{Atespace: "team-a", Name: "a-actor", CreateTime: now},
				ActorTemplateNamespace: "default",
				ActorTemplateName:      "tmpl",
			},
		},
	}
	w := New(Config{Lister: fake})
	snap, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	if len(snap.Actors) != 3 {
		t.Fatalf("expected 3 actors, got %d", len(snap.Actors))
	}
	if snap.Actors[0].Actor.GetMetadata().GetName() != "a-actor" {
		t.Errorf("expected first actor to be a-actor, got %s", snap.Actors[0].Actor.GetMetadata().GetName())
	}
	if snap.Actors[1].Actor.GetMetadata().GetName() != "b-actor" {
		t.Errorf("expected second actor to be b-actor, got %s", snap.Actors[1].Actor.GetMetadata().GetName())
	}
	if snap.Actors[2].Actor.GetMetadata().GetName() != "z-actor" {
		t.Errorf("expected third actor to be z-actor, got %s", snap.Actors[2].Actor.GetMetadata().GetName())
	}
}

func TestWatcher_ErrorResilience(t *testing.T) {
	fake := &fakeLister{
		actors: []*ateapipb.Actor{
			{Metadata: &ateapipb.ResourceMetadata{Atespace: "default", Name: "act1"}, Status: ateapipb.Actor_STATUS_RUNNING},
		},
	}
	w := New(Config{Lister: fake})
	snap1, err := w.Poll(context.Background())
	if err != nil {
		t.Fatalf("initial poll failed: %v", err)
	}
	if len(snap1.Actors) != 1 {
		t.Fatalf("expected 1 actor")
	}

	// Lister encounters network error
	fake.err = errors.New("connection refused")
	snap2, err2 := w.Poll(context.Background())
	if err2 == nil {
		t.Errorf("expected error, got nil")
	}
	if snap2.LastError == nil {
		t.Errorf("expected LastError to be set on snapshot")
	}
	// Should retain previous actors
	if len(snap2.Actors) != 1 {
		t.Errorf("expected previous actor state to be retained on error, got %d", len(snap2.Actors))
	}
}
