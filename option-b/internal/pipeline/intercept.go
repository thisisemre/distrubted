package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

const (
	interceptWorkers = 4
	interceptBufCap  = 30
	interceptTimeout = 2 * time.Second
)

// InterceptPlanEntry is the plan for one Nazgul unit.
type InterceptPlanEntry struct {
	UnitID       string  `json:"unitId"`
	TargetRegion string  `json:"targetRegion"`
	Score        float64 `json:"score"`
}

// InterceptPlan is the output of Pipeline 2.
type InterceptPlan struct {
	ByUnit []InterceptPlanEntry `json:"byUnit"`
}

// interceptWorkItem is one (Nazgul, route) pair.
type interceptWorkItem struct {
	unitID     string
	unitRegion string
	routeIdx   int
	cacheSnap  cache.WorldStateCache
}

// interceptResult is the output of one worker.
type interceptResult struct {
	unitID       string
	targetRegion string
	score        float64
}

// ComputeInterceptPlan runs Pipeline 2 with 4 workers.
func ComputeInterceptPlan(ctx context.Context, cacheSnap cache.WorldStateCache) InterceptPlan {
	ctx, cancel := context.WithTimeout(ctx, interceptTimeout)
	defer cancel()

	// Collect active Nazgul
	var nazgulUnits []game.UnitSnapshot
	for _, u := range cacheSnap.Units {
		cfg, ok := cacheSnap.UnitConfigs[u.ID]
		if !ok {
			continue
		}
		if cfg.Class == game.ClassNazgul && u.Status == game.UnitActive {
			nazgulUnits = append(nazgulUnits, u)
		}
	}

	if len(nazgulUnits) == 0 {
		return InterceptPlan{}
	}

	graph := game.NewGraph(pathConfigSlice(cacheSnap.PathConfigs))

	inputCh := make(chan interceptWorkItem, interceptBufCap)
	outputCh := make(chan interceptResult)

	var wg sync.WaitGroup

	// Dispatcher: one work item per (Nazgul, route-candidate) pair
	go func() {
		defer close(inputCh)
		for _, naz := range nazgulUnits {
			for i := range canonicalRoutes {
				select {
				case <-ctx.Done():
					return
				case inputCh <- interceptWorkItem{
					unitID:     naz.ID,
					unitRegion: naz.Region,
					routeIdx:   i,
					cacheSnap:  cacheSnap,
				}:
				}
			}
		}
	}()

	// 4 Workers
	for w := 0; w < interceptWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range inputCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				res := computeIntercept(item, graph)
				select {
				case outputCh <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Close output when workers done
	go func() {
		wg.Wait()
		close(outputCh)
	}()

	// Aggregator: best score per Nazgul
	bestByUnit := make(map[string]interceptResult)
	for res := range outputCh {
		if existing, ok := bestByUnit[res.unitID]; !ok || res.score > existing.score {
			bestByUnit[res.unitID] = res
		}
	}

	// Build output
	entries := make([]InterceptPlanEntry, 0, len(bestByUnit))
	for _, res := range bestByUnit {
		entries = append(entries, InterceptPlanEntry{
			UnitID:       res.unitID,
			TargetRegion: res.targetRegion,
			Score:        res.score,
		})
	}

	return InterceptPlan{ByUnit: entries}
}

func computeIntercept(item interceptWorkItem, graph *game.Graph) interceptResult {
	route := canonicalRoutes[item.routeIdx]
	routeLength := float64(len(route.Regions))
	bestScore := 0.0
	bestRegion := ""

	for _, regionID := range route.Regions {
		turnsToIntercept := float64(graph.Distance(item.unitRegion, regionID))
		if turnsToIntercept < 0 {
			continue
		}
		// RB turns to reach this region = sum of path costs up to this point
		rbTurnsToReach := float64(rbCostToRegion(route, regionID, item.cacheSnap))

		interceptWindow := rbTurnsToReach - turnsToIntercept
		var score float64
		if interceptWindow >= 0 {
			score = 1.0 - (turnsToIntercept / routeLength)
			if score < 0 {
				score = 0
			}
		} else {
			score = 0.0
		}

		if score > bestScore {
			bestScore = score
			bestRegion = regionID
		}
	}

	return interceptResult{
		unitID:       item.unitID,
		targetRegion: bestRegion,
		score:        bestScore,
	}
}

func rbCostToRegion(route struct {
	Name    string
	Regions []string
	Paths   []string
}, targetRegion string, snap cache.WorldStateCache) int {
	cost := 0
	for i, pathID := range route.Paths {
		pathCfg, ok := snap.PathConfigs[pathID]
		if !ok {
			cost++
		} else {
			cost += pathCfg.Cost
		}
		if i+1 < len(route.Regions) && route.Regions[i+1] == targetRegion {
			return cost
		}
	}
	return cost
}

func pathConfigSlice(m map[string]game.PathConfig) []game.PathConfig {
	result := make([]game.PathConfig, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
