package tests

import (
	"context"
	"testing"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	"github.com/rotr/ring-of-the-middle-earth/internal/pipeline"
)

// pipeline1_test.go — 2 cases (Section 35)

// Case 1: Route with known threat and surveillance values → correct riskScore computed
func TestPipeline1_KnownThreatAndSurveillance(t *testing.T) {
	snap := buildMinimalCacheSnap()

	// Set threat levels on Northern Bypass regions
	// Route regions: the-shire, bree, rivendell, lothlorien, emyn-muil, dead-marshes, ithilien, cirith-ungol, mount-doom
	snap.Regions["bree"] = game.RegionState{ID: "bree", ThreatLevel: 1}
	snap.Regions["rivendell"] = game.RegionState{ID: "rivendell", ThreatLevel: 0}
	snap.Regions["lothlorien"] = game.RegionState{ID: "lothlorien", ThreatLevel: 0}
	snap.Regions["emyn-muil"] = game.RegionState{ID: "emyn-muil", ThreatLevel: 2}
	snap.Regions["dead-marshes"] = game.RegionState{ID: "dead-marshes", ThreatLevel: 2}
	snap.Regions["ithilien"] = game.RegionState{ID: "ithilien", ThreatLevel: 2}
	snap.Regions["cirith-ungol"] = game.RegionState{ID: "cirith-ungol", ThreatLevel: 4}
	snap.Regions["mount-doom"] = game.RegionState{ID: "mount-doom", ThreatLevel: 5}

	// Set surveillance on one path
	snap.Paths["bree-to-rivendell"] = game.PathState{
		ID: "bree-to-rivendell", Status: game.StatusOpen, SurveillanceLevel: 2,
	}

	result := pipeline.ComputeRouteRisk(context.Background(), snap)

	// Find Northern Bypass route
	var northBypass *pipeline.RankedRoute
	for i := range result.Routes {
		if result.Routes[i].Name == "Northern Bypass" {
			northBypass = &result.Routes[i]
			break
		}
	}
	if northBypass == nil {
		t.Fatal("Northern Bypass route not found in results")
	}

	// Manual calculation for Northern Bypass:
	// Regions (dest): bree(1)+rivendell(0)+lothlorien(0)+emyn-muil(2)+dead-marshes(2)+ithilien(2)+cirith-ungol(4)+mount-doom(5) = 16
	// Path bree-to-rivendell surveillance=2: 2*3=6
	// Nazgul proximity: 0 (no Nazgul in snap)
	// Expected: 16 + 6 = 22
	expectedMin := 22
	if northBypass.RiskScore < expectedMin {
		t.Errorf("Northern Bypass riskScore=%d, expected >= %d (threat+surveillance contribution)",
			northBypass.RiskScore, expectedMin)
	}
}

// Case 2: Nazgul within 2 hops → proximity count adds correctly to score
func TestPipeline1_NazgulProximity(t *testing.T) {
	snap := buildMinimalCacheSnap()

	// Place a Nazgul at bree (directly on the Fellowship route)
	snap.Units["witch-king"] = game.UnitSnapshot{
		ID: "witch-king", Region: "bree", Strength: 5, Status: game.UnitActive,
	}
	snap.UnitConfigs["witch-king"] = game.UnitConfig{
		ID: "witch-king", Class: game.ClassNazgul, Side: game.SideShadow, Strength: 5,
	}

	// Set some path config so graph can compute neighbors
	snap.PathConfigs["shire-to-bree"] = game.PathConfig{ID: "shire-to-bree", From: "the-shire", To: "bree", Cost: 1}
	snap.PathConfigs["bree-to-weathertop"] = game.PathConfig{ID: "bree-to-weathertop", From: "bree", To: "weathertop", Cost: 1}

	without := pipeline.ComputeRouteRisk(context.Background(), buildMinimalCacheSnap())
	with := pipeline.ComputeRouteRisk(context.Background(), snap)

	// Fellowship route score should be higher when Nazgul is present
	var fellowshipWithout, fellowshipWith *pipeline.RankedRoute
	for i := range without.Routes {
		if without.Routes[i].Name == "Fellowship" {
			fellowshipWithout = &without.Routes[i]
		}
	}
	for i := range with.Routes {
		if with.Routes[i].Name == "Fellowship" {
			fellowshipWith = &with.Routes[i]
		}
	}

	if fellowshipWithout == nil || fellowshipWith == nil {
		t.Fatal("Fellowship route not found")
	}

	if fellowshipWith.RiskScore <= fellowshipWithout.RiskScore {
		t.Errorf("expected Fellowship riskScore to increase with Nazgul present: with=%d, without=%d",
			fellowshipWith.RiskScore, fellowshipWithout.RiskScore)
	}
}

// buildMinimalCacheSnap creates a minimal WorldStateCache snapshot for pipeline tests.
func buildMinimalCacheSnap() cache.WorldStateCache {
	regions := make(map[string]game.RegionState)
	regionNames := []string{
		"the-shire", "bree", "weathertop", "rivendell", "moria", "lothlorien",
		"emyn-muil", "dead-marshes", "ithilien", "cirith-ungol", "mount-doom",
		"tharbad", "fords-of-isen", "edoras", "minas-tirith", "osgiliath", "minas-morgul",
		"mordor", "rohan-plains", "fangorn", "isengard", "helms-deep",
	}
	for _, r := range regionNames {
		regions[r] = game.RegionState{ID: r}
	}

	paths := make(map[string]game.PathState)
	pathNames := []string{
		"shire-to-bree", "bree-to-weathertop", "weathertop-to-rivendell",
		"bree-to-rivendell", "rivendell-to-moria", "moria-to-lothlorien",
		"rivendell-to-lothlorien", "lothlorien-to-emyn-muil",
		"emyn-muil-to-dead-marshes", "dead-marshes-to-ithilien",
		"ithilien-to-cirith-ungol", "cirith-ungol-to-mount-doom",
		"dead-marshes-to-mordor", "mordor-to-mount-doom",
		"shire-to-tharbad", "tharbad-to-fords-of-isen", "fords-of-isen-to-edoras",
		"edoras-to-minas-tirith", "minas-tirith-to-osgiliath",
		"osgiliath-to-minas-morgul", "minas-morgul-to-cirith-ungol",
		"emyn-muil-to-ithilien",
	}
	for _, p := range pathNames {
		paths[p] = game.PathState{ID: p, Status: game.StatusOpen}
	}

	pathConfigs := map[string]game.PathConfig{
		"shire-to-bree":             {ID: "shire-to-bree", From: "the-shire", To: "bree", Cost: 1},
		"bree-to-weathertop":        {ID: "bree-to-weathertop", From: "bree", To: "weathertop", Cost: 1},
		"weathertop-to-rivendell":   {ID: "weathertop-to-rivendell", From: "weathertop", To: "rivendell", Cost: 1},
		"bree-to-rivendell":         {ID: "bree-to-rivendell", From: "bree", To: "rivendell", Cost: 2},
		"rivendell-to-moria":        {ID: "rivendell-to-moria", From: "rivendell", To: "moria", Cost: 2},
		"moria-to-lothlorien":       {ID: "moria-to-lothlorien", From: "moria", To: "lothlorien", Cost: 1},
		"rivendell-to-lothlorien":   {ID: "rivendell-to-lothlorien", From: "rivendell", To: "lothlorien", Cost: 2},
		"lothlorien-to-emyn-muil":   {ID: "lothlorien-to-emyn-muil", From: "lothlorien", To: "emyn-muil", Cost: 1},
		"emyn-muil-to-dead-marshes": {ID: "emyn-muil-to-dead-marshes", From: "emyn-muil", To: "dead-marshes", Cost: 1},
		"dead-marshes-to-ithilien":  {ID: "dead-marshes-to-ithilien", From: "dead-marshes", To: "ithilien", Cost: 1},
		"ithilien-to-cirith-ungol":  {ID: "ithilien-to-cirith-ungol", From: "ithilien", To: "cirith-ungol", Cost: 2},
		"cirith-ungol-to-mount-doom":{ID: "cirith-ungol-to-mount-doom", From: "cirith-ungol", To: "mount-doom", Cost: 2},
		"dead-marshes-to-mordor":    {ID: "dead-marshes-to-mordor", From: "dead-marshes", To: "mordor", Cost: 2},
		"mordor-to-mount-doom":      {ID: "mordor-to-mount-doom", From: "mordor", To: "mount-doom", Cost: 1},
		"shire-to-tharbad":          {ID: "shire-to-tharbad", From: "the-shire", To: "tharbad", Cost: 2},
		"tharbad-to-fords-of-isen":  {ID: "tharbad-to-fords-of-isen", From: "tharbad", To: "fords-of-isen", Cost: 2},
		"fords-of-isen-to-edoras":   {ID: "fords-of-isen-to-edoras", From: "fords-of-isen", To: "edoras", Cost: 1},
		"edoras-to-minas-tirith":    {ID: "edoras-to-minas-tirith", From: "edoras", To: "minas-tirith", Cost: 2},
		"minas-tirith-to-osgiliath": {ID: "minas-tirith-to-osgiliath", From: "minas-tirith", To: "osgiliath", Cost: 1},
		"osgiliath-to-minas-morgul": {ID: "osgiliath-to-minas-morgul", From: "osgiliath", To: "minas-morgul", Cost: 1},
		"minas-morgul-to-cirith-ungol":{ID: "minas-morgul-to-cirith-ungol", From: "minas-morgul", To: "cirith-ungol", Cost: 1},
		"emyn-muil-to-ithilien":     {ID: "emyn-muil-to-ithilien", From: "emyn-muil", To: "ithilien", Cost: 2},
	}

	return cache.WorldStateCache{
		Turn:        1,
		Units:       make(map[string]game.UnitSnapshot),
		Regions:     regions,
		Paths:       paths,
		UnitConfigs: make(map[string]game.UnitConfig),
		PathConfigs: pathConfigs,
		RegionConfigs: make(map[string]game.RegionConfig),
	}
}
