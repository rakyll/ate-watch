package watcher

import (
	"time"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
)

// ActorState tracks the live state of an individual actor.
type ActorState struct {
	Actor           *ateapipb.Actor
	PrevStatus      ateapipb.Actor_Status
	StatusChangedAt time.Time
}

// ActorKey creates the lookup key for an actor given its atespace and name.
func ActorKey(atespace, name string) string {
	return atespace + "/" + name
}

// Snapshot represents a point-in-time view of all watched actors.
type Snapshot struct {
	Timestamp     time.Time
	Actors        []*ActorState
	TotalCount    int
	CountByStatus map[ateapipb.Actor_Status]int
	LastError     error
}
