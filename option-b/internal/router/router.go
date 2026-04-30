package router

import (
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

// Event is a Kafka message routed between goroutines.
type Event struct {
	Topic   string
	Key     string
	Payload interface{}
}

// EventRouter is the single enforcement point for information asymmetry.
// The Dark Side NEVER receives the Ring Bearer's true position.
type EventRouter struct {
	lightSideSSECh chan<- Event
	darkSideSSECh  chan<- Event
	cacheUpdateCh  chan<- Event
	engineCh       chan<- Event
}

// NewEventRouter creates an EventRouter wired to the given channels.
func NewEventRouter(
	lightSSE chan<- Event,
	darkSSE chan<- Event,
	cacheUpdate chan<- Event,
	engine chan<- Event,
) *EventRouter {
	return &EventRouter{
		lightSideSSECh: lightSSE,
		darkSideSSECh:  darkSSE,
		cacheUpdateCh:  cacheUpdate,
		engineCh:       engine,
	}
}

// Route dispatches an incoming event to the appropriate channels.
// This is the ONLY place where information asymmetry is enforced.
func (r *EventRouter) Route(event Event) {
	switch event.Topic {

	case "game.ring.position":
		// Ring Bearer moved — Light Side only
		r.lightSideSSECh <- event
		r.cacheUpdateCh <- event
		// never darkSideSSECh

	case "game.ring.detection":
		// Detection events — Dark Side only
		r.darkSideSSECh <- event
		r.cacheUpdateCh <- event
		// never lightSideSSECh

	case "game.broadcast":
		// World state snapshot — both sides; RB position stripped before Dark Side delivery
		r.lightSideSSECh <- event
		r.darkSideSSECh <- stripRingBearer(event)
		r.cacheUpdateCh <- event

	case "game.events.unit",
		"game.events.region",
		"game.events.path":
		r.lightSideSSECh <- event
		r.darkSideSSECh <- event
		r.cacheUpdateCh <- event

	case "game.orders.validated":
		r.engineCh <- event
	}
}

// stripRingBearer returns a copy of the event with ring-bearer currentRegion set to "".
func stripRingBearer(event Event) Event {
	stripped := event
	if snapshot, ok := event.Payload.(game.WorldStateSnapshot); ok {
		// Deep copy units and strip ring bearer
		units := make([]game.UnitSnapshot, len(snapshot.Units))
		copy(units, snapshot.Units)
		for i, u := range units {
			if u.ID == "ring-bearer" {
				units[i].Region = ""
			}
		}
		stripped.Payload = game.WorldStateSnapshot{
			Turn:      snapshot.Turn,
			Units:     units,
			Regions:   snapshot.Regions,
			Timestamp: snapshot.Timestamp,
		}
	} else if m, ok := event.Payload.(map[string]interface{}); ok {
		// Copy map and strip
		newM := make(map[string]interface{}, len(m))
		for k, v := range m {
			newM[k] = v
		}
		if units, ok := newM["units"].([]map[string]interface{}); ok {
			for _, u := range units {
				if uid, ok := u["id"].(string); ok && uid == "ring-bearer" {
					u["region"] = ""
				}
			}
		}
		stripped.Payload = newM
	}
	return stripped
}
