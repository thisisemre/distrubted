package cache

import (
	"sync"

	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

// LightSideView holds Light Side exclusive state.
type LightSideView struct {
	RingBearerRegion string
	AssignedRoute    []string
	RouteIdx         int
}

// DarkSideView holds Dark Side state — RingBearerRegion is ALWAYS "".
type DarkSideView struct {
	RingBearerRegion   string // ALWAYS "" — never set by any code path
	LastDetectedRegion string
	LastDetectedTurn   int
}

// WorldStateCache is the in-memory cache of the full game state.
// CacheManager is the only goroutine that writes to it; workers receive value copies.
type WorldStateCache struct {
	mu sync.RWMutex

	Turn        int
	Units       map[string]game.UnitSnapshot
	Regions     map[string]game.RegionState
	Paths       map[string]game.PathState
	UnitConfigs map[string]game.UnitConfig  // read-only after startup
	PathConfigs map[string]game.PathConfig  // read-only after startup
	RegionConfigs map[string]game.RegionConfig // read-only after startup
	LightView   LightSideView
	DarkView    DarkSideView

	SarumanDisabled bool
	GameOver        bool
	Winner          game.Side
	RingBearerState game.RingBearerState // only accessible within game engine
}

// Snapshot returns a deep copy of the cache for safe use by worker goroutines.
func (c *WorldStateCache) Snapshot() WorldStateCache {
	c.mu.RLock()
	defer c.mu.RUnlock()

	units := make(map[string]game.UnitSnapshot, len(c.Units))
	for k, v := range c.Units {
		units[k] = v
	}
	regions := make(map[string]game.RegionState, len(c.Regions))
	for k, v := range c.Regions {
		regions[k] = v
	}
	paths := make(map[string]game.PathState, len(c.Paths))
	for k, v := range c.Paths {
		paths[k] = v
	}

	return WorldStateCache{
		Turn:            c.Turn,
		Units:           units,
		Regions:         regions,
		Paths:           paths,
		UnitConfigs:     c.UnitConfigs,
		PathConfigs:     c.PathConfigs,
		RegionConfigs:   c.RegionConfigs,
		LightView:       c.LightView,
		DarkView:        DarkSideView{RingBearerRegion: "", LastDetectedRegion: c.DarkView.LastDetectedRegion, LastDetectedTurn: c.DarkView.LastDetectedTurn},
		SarumanDisabled: c.SarumanDisabled,
		GameOver:        c.GameOver,
		Winner:          c.Winner,
		RingBearerState: c.RingBearerState,
	}
}

// Update applies a turn result to the cache.
func (c *WorldStateCache) Update(result game.TurnResult, lightRegion string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Turn = result.Turn + 1

	for _, u := range result.Snapshot.Units {
		c.Units[u.ID] = u
	}
	for _, r := range result.Snapshot.Regions {
		c.Regions[r.ID] = r
	}

	if result.Winner != "" {
		c.GameOver = true
		c.Winner = result.Winner
	}

	// Update Light Side view with true region
	c.LightView.RingBearerRegion = lightRegion

	// DarkView.RingBearerRegion is NEVER set
	if result.DetectionResult.Exposed {
		c.DarkView.LastDetectedRegion = result.DetectionResult.DetectedInRegion
		c.DarkView.LastDetectedTurn = result.Turn
	}
}

// UpdatePaths updates path state in cache.
func (c *WorldStateCache) UpdatePaths(paths map[string]game.PathState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range paths {
		c.Paths[k] = v
	}
}

// SetSarumanDisabled marks Saruman permanently disabled.
func (c *WorldStateCache) SetSarumanDisabled() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SarumanDisabled = true
}

// GetPublicState returns the world state safe for a given player (strips RB position for dark side).
func (c *WorldStateCache) GetPublicState(playerID string) map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	units := make([]map[string]interface{}, 0, len(c.Units))
	for _, u := range c.Units {
		entry := map[string]interface{}{
			"id":       u.ID,
			"region":   u.Region,
			"strength": u.Strength,
			"status":   u.Status,
		}
		// Strip ring-bearer region for dark side
		if c.UnitConfigs[u.ID].Class == game.ClassRingBearer && playerID == game.PlayerDarkSide {
			entry["region"] = ""
		}
		units = append(units, entry)
	}

	regions := make([]map[string]interface{}, 0, len(c.Regions))
	for _, r := range c.Regions {
		regions = append(regions, map[string]interface{}{
			"id":           r.ID,
			"controlledBy": r.ControlledBy,
			"threatLevel":  r.ThreatLevel,
			"fortified":    r.Fortified,
		})
	}

	paths := make([]map[string]interface{}, 0, len(c.Paths))
	for _, p := range c.Paths {
		paths = append(paths, map[string]interface{}{
			"id":                p.ID,
			"status":            p.Status,
			"surveillanceLevel": p.SurveillanceLevel,
		})
	}

	state := map[string]interface{}{
		"turn":    c.Turn,
		"units":   units,
		"regions": regions,
		"paths":   paths,
	}

	if playerID == game.PlayerLightSide {
		state["ringBearer"] = map[string]interface{}{
			"currentRegion": c.LightView.RingBearerRegion,
		}
	} else {
		state["ringBearer"] = map[string]interface{}{
			"currentRegion":      "",
			"lastDetectedRegion": c.DarkView.LastDetectedRegion,
			"lastDetectedTurn":   c.DarkView.LastDetectedTurn,
		}
	}

	return state
}
