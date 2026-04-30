package tests

import (
	"testing"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	"github.com/rotr/ring-of-the-middle-earth/internal/router"
)

// router_test.go — 3 cases with -race (Section 35)

// Case 1: WorldStateSnapshot with ring-bearer region set →
//   Dark Side receives currentRegion="", Light Side receives real value
func TestRouter_DarkSideNeverSeesRingBearerRegion(t *testing.T) {
	lightCh := make(chan router.Event, 10)
	darkCh := make(chan router.Event, 10)
	cacheCh := make(chan router.Event, 10)
	engineCh := make(chan router.Event, 10)

	r := router.NewEventRouter(lightCh, darkCh, cacheCh, engineCh)

	snapshot := game.WorldStateSnapshot{
		Turn: 5,
		Units: []game.UnitSnapshot{
			{ID: "ring-bearer", Region: "rivendell", Strength: 1, Status: game.UnitActive},
			{ID: "aragorn", Region: "bree", Strength: 5, Status: game.UnitActive},
		},
	}

	event := router.Event{
		Topic:   "game.broadcast",
		Key:     "world",
		Payload: snapshot,
	}

	r.Route(event)

	lightEvent := <-lightCh
	darkEvent := <-darkCh

	// Light Side gets real region
	lightSnap, ok := lightEvent.Payload.(game.WorldStateSnapshot)
	if !ok {
		t.Fatalf("light side payload is not WorldStateSnapshot")
	}
	for _, u := range lightSnap.Units {
		if u.ID == "ring-bearer" {
			if u.Region != "rivendell" {
				t.Errorf("light side: expected ring-bearer region=rivendell, got %q", u.Region)
			}
		}
	}

	// Dark Side gets "" for ring bearer
	darkSnap, ok := darkEvent.Payload.(game.WorldStateSnapshot)
	if !ok {
		t.Fatalf("dark side payload is not WorldStateSnapshot")
	}
	for _, u := range darkSnap.Units {
		if u.ID == "ring-bearer" {
			if u.Region != "" {
				t.Errorf("dark side: ring-bearer region must be empty string, got %q", u.Region)
			}
		}
	}
}

// Case 2: RingBearerMoved event → never reaches Dark Side SSE channel
func TestRouter_RingBearerMovedNeverReachesDarkSide(t *testing.T) {
	lightCh := make(chan router.Event, 10)
	darkCh := make(chan router.Event, 10)
	cacheCh := make(chan router.Event, 10)
	engineCh := make(chan router.Event, 10)

	r := router.NewEventRouter(lightCh, darkCh, cacheCh, engineCh)

	event := router.Event{
		Topic:   "game.ring.position",
		Key:     "ring-bearer",
		Payload: map[string]interface{}{"trueRegion": "rivendell", "turn": 3},
	}

	r.Route(event)

	// Light side should receive it
	select {
	case <-lightCh:
		// correct
	default:
		t.Error("light side should receive ring.position event")
	}

	// Dark side should NOT receive it
	select {
	case ev := <-darkCh:
		t.Errorf("dark side should NEVER receive ring.position, got topic=%s", ev.Topic)
	default:
		// correct — channel is empty
	}
}

// Case 3: cache.DarkView.RingBearerRegion is always "" after any cache update
func TestRouter_DarkViewRingBearerRegionAlwaysEmpty(t *testing.T) {
	c := &cache.WorldStateCache{
		Turn:  1,
		Units: make(map[string]game.UnitSnapshot),
		Regions: make(map[string]game.RegionState),
		Paths: make(map[string]game.PathState),
		UnitConfigs: map[string]game.UnitConfig{
			"ring-bearer": {ID: "ring-bearer", Class: game.ClassRingBearer, Side: game.SideFreePeoples, Strength: 1},
		},
		PathConfigs:   make(map[string]game.PathConfig),
		RegionConfigs: make(map[string]game.RegionConfig),
	}

	// Simulate multiple updates with real ring bearer position
	for i := 0; i < 10; i++ {
		result := game.TurnResult{
			Turn: i + 1,
			Snapshot: game.WorldStateSnapshot{
				Turn: i + 1,
				Units: []game.UnitSnapshot{
					{ID: "ring-bearer", Region: "rivendell", Strength: 1, Status: game.UnitActive},
				},
			},
		}
		c.Update(result, "rivendell")

		snap := c.Snapshot()
		if snap.DarkView.RingBearerRegion != "" {
			t.Errorf("turn %d: DarkView.RingBearerRegion must always be empty string, got %q",
				i+1, snap.DarkView.RingBearerRegion)
		}
	}
}
