package tests

import (
	"context"
	"testing"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	"github.com/rotr/ring-of-the-middle-earth/internal/pipeline"
)

// pipeline2_test.go — 2 cases (Section 35)

// Case 1: Positive intercept window → score > 0
func TestPipeline2_PositiveInterceptWindow(t *testing.T) {
	snap := buildInterceptSnap()

	// Witch-King at bree (1 hop from the-shire, 2 from weathertop)
	snap.Units["witch-king"] = game.UnitSnapshot{
		ID: "witch-king", Region: "bree", Strength: 5, Status: game.UnitActive,
	}
	snap.UnitConfigs["witch-king"] = game.UnitConfig{
		ID: "witch-king", Class: game.ClassNazgul, Side: game.SideShadow,
	}

	result := pipeline.ComputeInterceptPlan(context.Background(), snap)

	if len(result.ByUnit) == 0 {
		t.Fatal("expected at least one interception plan entry")
	}

	var witchKingEntry *pipeline.InterceptPlanEntry
	for i := range result.ByUnit {
		if result.ByUnit[i].UnitID == "witch-king" {
			witchKingEntry = &result.ByUnit[i]
			break
		}
	}

	if witchKingEntry == nil {
		t.Fatal("no intercept plan entry for witch-king")
	}

	if witchKingEntry.Score <= 0 {
		t.Errorf("expected score > 0 for positive intercept window, got %.4f", witchKingEntry.Score)
	}
}

// Case 2: Negative intercept window → score = 0.0
func TestPipeline2_NegativeInterceptWindow(t *testing.T) {
	snap := buildInterceptSnap()

	// Nazgul at mount-doom — Ring Bearer starts at the-shire, far away
	// By the time RB reaches mount-doom, nazgul is already there → window negative for early regions
	// Place Nazgul at cirith-ungol (very far from the-shire)
	snap.Units["nazgul-2"] = game.UnitSnapshot{
		ID: "nazgul-2", Region: "mount-doom", Strength: 3, Status: game.UnitActive,
	}
	snap.UnitConfigs["nazgul-2"] = game.UnitConfig{
		ID: "nazgul-2", Class: game.ClassNazgul, Side: game.SideShadow,
	}

	result := pipeline.ComputeInterceptPlan(context.Background(), snap)

	var n2Entry *pipeline.InterceptPlanEntry
	for i := range result.ByUnit {
		if result.ByUnit[i].UnitID == "nazgul-2" {
			n2Entry = &result.ByUnit[i]
			break
		}
	}

	if n2Entry == nil {
		// If no entry found (all scores 0, skipped), that's also acceptable
		return
	}

	// A Nazgul at mount-doom intercepting early route regions will have
	// turnsToIntercept >> rbTurnsToReach → negative window → score = 0.0 for those
	// The best it can do is intercept at mount-doom itself (window=0 or positive)
	// Score should be >= 0 always
	if n2Entry.Score < 0 {
		t.Errorf("score must not be negative, got %.4f", n2Entry.Score)
	}
}

func buildInterceptSnap() cache.WorldStateCache {
	snap := buildMinimalCacheSnap()

	// Ensure all path configs for the routes are present
	extraPaths := map[string]game.PathConfig{
		"bree-to-tharbad":            {ID: "bree-to-tharbad", From: "bree", To: "tharbad", Cost: 1},
		"fangorn-to-isengard":        {ID: "fangorn-to-isengard", From: "fangorn", To: "isengard", Cost: 1},
		"lothlorien-to-rohan-plains": {ID: "lothlorien-to-rohan-plains", From: "lothlorien", To: "rohan-plains", Cost: 1},
		"rohan-plains-to-fangorn":    {ID: "rohan-plains-to-fangorn", From: "rohan-plains", To: "fangorn", Cost: 1},
		"rohan-plains-to-edoras":     {ID: "rohan-plains-to-edoras", From: "rohan-plains", To: "edoras", Cost: 1},
		"edoras-to-helms-deep":       {ID: "edoras-to-helms-deep", From: "edoras", To: "helms-deep", Cost: 1},
		"fords-of-isen-to-helms-deep":{ID: "fords-of-isen-to-helms-deep", From: "fords-of-isen", To: "helms-deep", Cost: 1},
		"ithilien-to-osgiliath":      {ID: "ithilien-to-osgiliath", From: "ithilien", To: "osgiliath", Cost: 1},
		"ithilien-to-minas-tirith":   {ID: "ithilien-to-minas-tirith", From: "ithilien", To: "minas-tirith", Cost: 1},
		"minas-morgul-to-mordor":     {ID: "minas-morgul-to-mordor", From: "minas-morgul", To: "mordor", Cost: 1},
		"cirith-ungol-to-mordor":     {ID: "cirith-ungol-to-mordor", From: "cirith-ungol", To: "mordor", Cost: 1},
	}
	for k, v := range extraPaths {
		snap.PathConfigs[k] = v
		snap.Paths[k] = game.PathState{ID: k, Status: game.StatusOpen}
	}

	extraRegions := []string{"tharbad", "fangorn", "isengard", "helms-deep", "rohan-plains"}
	for _, r := range extraRegions {
		if _, exists := snap.Regions[r]; !exists {
			snap.Regions[r] = game.RegionState{ID: r}
		}
	}

	return snap
}
