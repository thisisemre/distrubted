package game

// Graph provides BFS/shortest-path operations on the map.
type Graph struct {
	// adjacency maps regionID -> []PathConfig (both directions)
	adjacency map[string][]PathConfig
	paths     map[string]PathConfig
}

// NewGraph builds the graph from path configs (bidirectional).
func NewGraph(pathCfgs []PathConfig) *Graph {
	g := &Graph{
		adjacency: make(map[string][]PathConfig),
		paths:     make(map[string]PathConfig),
	}
	for _, p := range pathCfgs {
		g.paths[p.ID] = p
		g.adjacency[p.From] = append(g.adjacency[p.From], p)
		// Add reverse direction
		rev := PathConfig{ID: p.ID, From: p.To, To: p.From, Cost: p.Cost}
		g.adjacency[p.To] = append(g.adjacency[p.To], rev)
	}
	return g
}

// Distance returns the minimum hop count between two regions (ignoring path cost).
// Returns -1 if no path exists.
func (g *Graph) Distance(from, to string) int {
	if from == to {
		return 0
	}
	visited := map[string]bool{from: true}
	queue := []string{from}
	dist := 0
	for len(queue) > 0 {
		dist++
		next := []string{}
		for _, cur := range queue {
			for _, edge := range g.adjacency[cur] {
				if edge.To == to {
					return dist
				}
				if !visited[edge.To] {
					visited[edge.To] = true
					next = append(next, edge.To)
				}
			}
		}
		queue = next
	}
	return -1
}

// ShortestPathCost returns the minimum total cost path between two regions.
// Uses Dijkstra with path costs.
func (g *Graph) ShortestPathCost(from, to string) int {
	if from == to {
		return 0
	}
	dist := map[string]int{from: 0}
	// Simple Dijkstra with priority queue emulated as repeated min-scan
	unvisited := map[string]bool{from: true}
	for reg := range g.adjacency {
		if reg != from {
			dist[reg] = 1<<31 - 1
			unvisited[reg] = true
		}
	}
	for len(unvisited) > 0 {
		// Find unvisited node with minimum dist
		cur := ""
		minD := 1<<31 - 1
		for r := range unvisited {
			if d, ok := dist[r]; ok && d < minD {
				minD = d
				cur = r
			}
		}
		if cur == "" || minD == 1<<31-1 {
			break
		}
		delete(unvisited, cur)
		if cur == to {
			return dist[to]
		}
		for _, edge := range g.adjacency[cur] {
			newDist := dist[cur] + edge.Cost
			if existing, ok := dist[edge.To]; !ok || newDist < existing {
				dist[edge.To] = newDist
			}
		}
	}
	if d, ok := dist[to]; ok && d < 1<<31-1 {
		return d
	}
	return -1
}

// RegionsWithinHops returns all region IDs reachable within `hops` hops (hop count, not cost).
func (g *Graph) RegionsWithinHops(from string, hops int) []string {
	visited := map[string]bool{from: true}
	queue := []string{from}
	result := []string{}
	for h := 0; h < hops && len(queue) > 0; h++ {
		next := []string{}
		for _, cur := range queue {
			for _, edge := range g.adjacency[cur] {
				if !visited[edge.To] {
					visited[edge.To] = true
					next = append(next, edge.To)
					result = append(result, edge.To)
				}
			}
		}
		queue = next
	}
	return result
}

// Neighbors returns the directly adjacent region IDs.
func (g *Graph) Neighbors(region string) []string {
	neighbors := []string{}
	for _, edge := range g.adjacency[region] {
		neighbors = append(neighbors, edge.To)
	}
	return neighbors
}

// PathsFromRegion returns all paths whose endpoint includes the given region.
func (g *Graph) PathsFromRegion(region string, allPaths map[string]PathConfig) []PathConfig {
	result := []PathConfig{}
	for _, p := range allPaths {
		if p.From == region || p.To == region {
			result = append(result, p)
		}
	}
	return result
}

// PathEndpoints returns the two endpoint regions of a path.
func PathEndpoints(p PathConfig) [2]string {
	return [2]string{p.From, p.To}
}
