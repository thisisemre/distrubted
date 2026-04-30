package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

const (
	routeRiskWorkers = 4
	routeRiskBufCap  = 20
	routeRiskTimeout = 2 * time.Second
)

// RankedRoute is a canonical route with its computed risk score.
type RankedRoute struct {
	Name            string   `json:"name"`
	Regions         []string `json:"regions"`
	Paths           []string `json:"paths"`
	RiskScore       int      `json:"riskScore"`
	ThreatenedPaths []string `json:"threatenedPaths"`
	BlockedPaths    []string `json:"blockedPaths"`
}

// RankedRouteList is the output of Pipeline 1.
type RankedRouteList struct {
	Routes      []RankedRoute `json:"routes"`
	Recommended string        `json:"recommended"`
	Warnings    []string      `json:"warnings"`
}

// canonicalRoutes defines the 4 canonical Ring Bearer routes from Section 2.3.
var canonicalRoutes = []struct {
	Name    string
	Regions []string
	Paths   []string
}{
	{
		Name:    "Fellowship",
		Regions: []string{"the-shire", "bree", "weathertop", "rivendell", "moria", "lothlorien", "emyn-muil", "ithilien", "cirith-ungol", "mount-doom"},
		Paths:   []string{"shire-to-bree", "bree-to-weathertop", "weathertop-to-rivendell", "rivendell-to-moria", "moria-to-lothlorien", "lothlorien-to-emyn-muil", "emyn-muil-to-ithilien", "ithilien-to-cirith-ungol", "cirith-ungol-to-mount-doom"},
	},
	{
		Name:    "Northern Bypass",
		Regions: []string{"the-shire", "bree", "rivendell", "lothlorien", "emyn-muil", "dead-marshes", "ithilien", "cirith-ungol", "mount-doom"},
		Paths:   []string{"shire-to-bree", "bree-to-rivendell", "rivendell-to-lothlorien", "lothlorien-to-emyn-muil", "emyn-muil-to-dead-marshes", "dead-marshes-to-ithilien", "ithilien-to-cirith-ungol", "cirith-ungol-to-mount-doom"},
	},
	{
		Name:    "Dark Route",
		Regions: []string{"the-shire", "bree", "rivendell", "lothlorien", "emyn-muil", "dead-marshes", "mordor", "mount-doom"},
		Paths:   []string{"shire-to-bree", "bree-to-rivendell", "rivendell-to-lothlorien", "lothlorien-to-emyn-muil", "emyn-muil-to-dead-marshes", "dead-marshes-to-mordor", "mordor-to-mount-doom"},
	},
	{
		Name:    "Southern Corridor",
		Regions: []string{"the-shire", "tharbad", "fords-of-isen", "edoras", "minas-tirith", "osgiliath", "minas-morgul", "cirith-ungol", "mount-doom"},
		Paths:   []string{"shire-to-tharbad", "tharbad-to-fords-of-isen", "fords-of-isen-to-edoras", "edoras-to-minas-tirith", "minas-tirith-to-osgiliath", "osgiliath-to-minas-morgul", "minas-morgul-to-cirith-ungol", "cirith-ungol-to-mount-doom"},
	},
}

// routeRiskInput is the work item sent to workers.
type routeRiskInput struct {
	route      int // index into canonicalRoutes
	cacheSnap  cache.WorldStateCache
}

// routeRiskOutput is the result from a worker.
type routeRiskOutput struct {
	ranked RankedRoute
	err    error
}

// ComputeRouteRisk runs Pipeline 1 with 4 workers and returns ranked routes.
// Uses context for cancellation and times out after 2 seconds.
func ComputeRouteRisk(ctx context.Context, cacheSnap cache.WorldStateCache) RankedRouteList {
	ctx, cancel := context.WithTimeout(ctx, routeRiskTimeout)
	defer cancel()

	inputCh := make(chan routeRiskInput, routeRiskBufCap)
	outputCh := make(chan routeRiskOutput)

	var wg sync.WaitGroup

	// Dispatcher
	go func() {
		defer close(inputCh)
		for i := range canonicalRoutes {
			select {
			case <-ctx.Done():
				return
			case inputCh <- routeRiskInput{route: i, cacheSnap: cacheSnap}:
			}
		}
	}()

	// 4 Workers
	for w := 0; w < routeRiskWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range inputCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				ranked := computeSingleRouteRisk(item.route, item.cacheSnap)
				select {
				case outputCh <- routeRiskOutput{ranked: ranked}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Close output when all workers done
	go func() {
		wg.Wait()
		close(outputCh)
	}()

	// Aggregator
	var results []RankedRoute
	for out := range outputCh {
		if out.err == nil {
			results = append(results, out.ranked)
		}
	}

	// Sort by risk score ascending (lower = better)
	sortRoutesByRisk(results)

	recommended := ""
	if len(results) > 0 {
		recommended = results[0].Name
	}

	var warnings []string
	for _, r := range results {
		if len(r.BlockedPaths) > 0 {
			warnings = append(warnings, r.Name+": has blocked paths")
		}
	}

	return RankedRouteList{
		Routes:      results,
		Recommended: recommended,
		Warnings:    warnings,
	}
}

func computeSingleRouteRisk(routeIdx int, snap cache.WorldStateCache) RankedRoute {
	route := canonicalRoutes[routeIdx]
	score := 0
	var threatenedPaths, blockedPaths []string

	// Sum region threat levels (destination regions only)
	for _, regionID := range route.Regions[1:] {
		if r, ok := snap.Regions[regionID]; ok {
			score += r.ThreatLevel
		}
	}

	// Sum path surveillance * 3, count threatened/blocked
	for _, pathID := range route.Paths {
		if p, ok := snap.Paths[pathID]; ok {
			score += p.SurveillanceLevel * 3
			switch p.Status {
			case game.StatusThreatened:
				score += 2
				threatenedPaths = append(threatenedPaths, pathID)
			case game.StatusBlocked:
				score += 5
				blockedPaths = append(blockedPaths, pathID)
			}
		}
	}

	// Nazgul proximity count: Nazgul within 2 hops of any region in route
	nazgulProximity := countNazgulProximity(route.Regions, snap)
	score += nazgulProximity * 2

	return RankedRoute{
		Name:            route.Name,
		Regions:         route.Regions,
		Paths:           route.Paths,
		RiskScore:       score,
		ThreatenedPaths: threatenedPaths,
		BlockedPaths:    blockedPaths,
	}
}

func countNazgulProximity(routeRegions []string, snap cache.WorldStateCache) int {
	// Build set of regions within 2 hops of any route region
	nearRegions := make(map[string]bool)
	for _, r := range routeRegions {
		nearRegions[r] = true
	}

	// Get adjacency from path configs
	adj := buildAdjacency(snap.PathConfigs)

	for _, r := range routeRegions {
		// 1 hop
		for _, n1 := range adj[r] {
			nearRegions[n1] = true
			// 2 hops
			for _, n2 := range adj[n1] {
				nearRegions[n2] = true
			}
		}
	}

	count := 0
	for _, u := range snap.Units {
		cfg, ok := snap.UnitConfigs[u.ID]
		if !ok {
			continue
		}
		if cfg.Class == game.ClassNazgul && u.Status == game.UnitActive {
			if nearRegions[u.Region] {
				count++
			}
		}
	}
	return count
}

func buildAdjacency(pathConfigs map[string]game.PathConfig) map[string][]string {
	adj := make(map[string][]string)
	for _, p := range pathConfigs {
		adj[p.From] = append(adj[p.From], p.To)
		adj[p.To] = append(adj[p.To], p.From)
	}
	return adj
}

func sortRoutesByRisk(routes []RankedRoute) {
	for i := 1; i < len(routes); i++ {
		for j := i; j > 0 && routes[j].RiskScore < routes[j-1].RiskScore; j-- {
			routes[j], routes[j-1] = routes[j-1], routes[j]
		}
	}
}
