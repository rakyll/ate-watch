package watcher

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/rakyll/ate-watch/internal/client"
	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
)

// Config defines the configuration for the actor watcher.
type Config struct {
	Lister       client.ActorLister
	Atespace     string
	Interval     time.Duration
	StatusFilter []ateapipb.Actor_Status
}

// Watcher monitors actors on the Substrate control plane and tracks status transitions.
type Watcher struct {
	cfg         Config
	mu          sync.RWMutex
	actors      map[string]*ActorState
	initialized bool
}

// New creates a new Watcher with the given configuration.
func New(cfg Config) *Watcher {
	if cfg.Interval <= 0 {
		cfg.Interval = 500 * time.Millisecond
	}

	return &Watcher{
		cfg:    cfg,
		actors: make(map[string]*ActorState),
	}
}

// Poll fetches the latest actor list from the control plane and returns the updated Snapshot.
func (w *Watcher) Poll(ctx context.Context) (*Snapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	rawActors, err := w.cfg.Lister.ListAllActors(ctx, w.cfg.Atespace)
	if err != nil {
		return &Snapshot{
			Timestamp:     now,
			Actors:        w.actorListLocked(),
			TotalCount:    len(w.actors),
			CountByStatus: w.countByStatusLocked(),
			LastError:     err,
		}, err
	}

	seen := make(map[string]bool, len(rawActors))

	for _, actor := range rawActors {
		if actor == nil || actor.GetMetadata() == nil {
			continue
		}

		key := ActorKey(actor.GetMetadata().GetAtespace(), actor.GetMetadata().GetName())
		seen[key] = true

		existing, exists := w.actors[key]
		if !exists {
			// New actor discovered
			state := &ActorState{
				Actor:           actor,
				PrevStatus:      actor.GetStatus(),
				StatusChangedAt: now,
			}
			w.actors[key] = state
		} else {
			// Existing actor - check for changes
			oldActor := existing.Actor

			// Status transition check
			if oldActor.GetStatus() != actor.GetStatus() {
				existing.PrevStatus = oldActor.GetStatus()
				existing.StatusChangedAt = now
			}

			existing.Actor = actor
		}
	}

	// Detect deleted actors
	for key := range w.actors {
		if !seen[key] {
			delete(w.actors, key)
		}
	}

	w.initialized = true

	actorsList := w.actorListLocked()
	// Apply status filters if configured
	if len(w.cfg.StatusFilter) > 0 {
		filterMap := make(map[ateapipb.Actor_Status]bool, len(w.cfg.StatusFilter))
		for _, s := range w.cfg.StatusFilter {
			filterMap[s] = true
		}
		var filtered []*ActorState
		for _, s := range actorsList {
			if filterMap[s.Actor.GetStatus()] {
				filtered = append(filtered, s)
			}
		}
		actorsList = filtered
	}

	return &Snapshot{
		Timestamp:     now,
		Actors:        actorsList,
		TotalCount:    len(w.actors),
		CountByStatus: w.countByStatusLocked(),
	}, nil
}

// Watch runs a continuous watch loop until the context is cancelled, emitting Snapshots.
func (w *Watcher) Watch(ctx context.Context) (<-chan *Snapshot, <-chan error) {
	snapCh := make(chan *Snapshot, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(snapCh)
		defer close(errCh)

		ticker := time.NewTicker(w.cfg.Interval)
		defer ticker.Stop()

		// Initial poll immediately
		snap, err := w.Poll(ctx)
		if err != nil {
			errCh <- err
		}
		if snap != nil {
			snapCh <- snap
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				snap, err := w.Poll(ctx)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
				if snap != nil {
					select {
					case snapCh <- snap:
					default:
					}
				}
			}
		}
	}()

	return snapCh, errCh
}

func (w *Watcher) actorListLocked() []*ActorState {
	list := make([]*ActorState, 0, len(w.actors))
	for _, s := range w.actors {
		list = append(list, s)
	}

	// Sort deterministically: Atespace, Template Namespace, Template Name, Actor Name
	slices.SortFunc(list, func(a, b *ActorState) int {
		if c := cmp.Compare(a.Actor.GetMetadata().GetAtespace(), b.Actor.GetMetadata().GetAtespace()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Actor.GetActorTemplateNamespace(), b.Actor.GetActorTemplateNamespace()); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Actor.GetActorTemplateName(), b.Actor.GetActorTemplateName()); c != 0 {
			return c
		}
		return cmp.Compare(a.Actor.GetMetadata().GetName(), b.Actor.GetMetadata().GetName())
	})

	return list
}

func (w *Watcher) countByStatusLocked() map[ateapipb.Actor_Status]int {
	counts := make(map[ateapipb.Actor_Status]int)
	for _, s := range w.actors {
		counts[s.Actor.GetStatus()]++
	}
	return counts
}
