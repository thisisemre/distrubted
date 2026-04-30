package tests

import (
	"testing"

	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

// Helper to build minimal unit config
func mkCfg(id string, side game.Side, str int, leadership bool, leaderBonus int, ignoresFortress bool, indestructible bool, respawns bool, respawnTurns int) game.UnitConfig {
	return game.UnitConfig{
		ID:              id,
		Side:            side,
		Strength:        str,
		Leadership:      leadership,
		LeadershipBonus: leaderBonus,
		IgnoresFortress: ignoresFortress,
		Indestructible:  indestructible,
		Respawns:        respawns,
		RespawnTurns:    respawnTurns,
	}
}

func mkSnap(id string, region string, str int, status game.UnitStatus) game.UnitSnapshot {
	return game.UnitSnapshot{ID: id, Region: region, Strength: str, Status: status}
}

// combat_test.go — 6 cases (Section 35)

// Case 1: Attacker(5) vs Defender(5, PLAINS) → tie
func TestCombat_TiePlains(t *testing.T) {
	attackers := []game.AttackerUnit{{
		Snapshot: mkSnap("a1", "bree", 5, game.UnitActive),
		Config:   mkCfg("a1", game.SideShadow, 5, false, 0, false, false, false, 0),
	}}
	defenders := []game.DefenderUnit{{
		Snapshot: mkSnap("d1", "bree", 5, game.UnitActive),
		Config:   mkCfg("d1", game.SideFreePeoples, 5, false, 0, false, false, false, 0),
	}}
	region := game.RegionState{ControlledBy: game.SideFreePeoples}
	regionCfg := game.RegionConfig{Terrain: game.TerrainPlains}

	result := game.ResolveCombat(attackers, defenders, region, regionCfg)
	if result.AttackerWon {
		t.Errorf("expected tie (defender holds), got attacker won")
	}
	if result.Damage != 1 {
		t.Errorf("expected each attacker loses 1, got damage=%d", result.Damage)
	}
}

// Case 2: Attacker(5) vs Defender(5, FORTRESS) → defender wins
func TestCombat_FortressTerrain(t *testing.T) {
	attackers := []game.AttackerUnit{{
		Snapshot: mkSnap("a1", "isengard", 5, game.UnitActive),
		Config:   mkCfg("a1", game.SideShadow, 5, false, 0, false, false, false, 0),
	}}
	defenders := []game.DefenderUnit{{
		Snapshot: mkSnap("d1", "isengard", 5, game.UnitActive),
		Config:   mkCfg("d1", game.SideFreePeoples, 5, false, 0, false, false, false, 0),
	}}
	region := game.RegionState{ControlledBy: game.SideFreePeoples}
	regionCfg := game.RegionConfig{Terrain: game.TerrainFortress}

	result := game.ResolveCombat(attackers, defenders, region, regionCfg)
	if result.AttackerWon {
		t.Errorf("expected defender wins with fortress bonus (5 vs 7), got attacker won")
	}
}

// Case 3: UrukHai(5, ignoresFortress) vs Defender(5, FORTRESS) → tie (5 vs 5)
func TestCombat_UrukHaiIgnoresFortressTerrain(t *testing.T) {
	attackers := []game.AttackerUnit{{
		Snapshot: mkSnap("uruk", "isengard", 5, game.UnitActive),
		Config:   mkCfg("uruk", game.SideShadow, 5, false, 0, true, false, false, 0),
	}}
	defenders := []game.DefenderUnit{{
		Snapshot: mkSnap("d1", "isengard", 5, game.UnitActive),
		Config:   mkCfg("d1", game.SideFreePeoples, 5, false, 0, false, false, false, 0),
	}}
	region := game.RegionState{ControlledBy: game.SideFreePeoples, Fortified: false}
	regionCfg := game.RegionConfig{Terrain: game.TerrainFortress}

	result := game.ResolveCombat(attackers, defenders, region, regionCfg)
	// 5 vs 5 (terrain skipped) → tie
	if result.AttackerWon {
		t.Errorf("expected tie (ignoresFortress skips terrain bonus), got attacker won")
	}
}

// Case 4: UrukHai(5) vs Defender(5, FORTRESS, fortified) → defender wins (5 vs 7)
func TestCombat_UrukHaiVsFortified(t *testing.T) {
	attackers := []game.AttackerUnit{{
		Snapshot: mkSnap("uruk", "minas-tirith", 5, game.UnitActive),
		Config:   mkCfg("uruk", game.SideShadow, 5, false, 0, true, false, false, 0),
	}}
	defenders := []game.DefenderUnit{{
		Snapshot: mkSnap("gondor", "minas-tirith", 5, game.UnitActive),
		Config:   mkCfg("gondor", game.SideFreePeoples, 5, false, 0, false, false, false, 0),
	}}
	region := game.RegionState{ControlledBy: game.SideFreePeoples, Fortified: true}
	regionCfg := game.RegionConfig{Terrain: game.TerrainFortress}

	result := game.ResolveCombat(attackers, defenders, region, regionCfg)
	// ignoresFortress skips terrain but NOT fortification: 5 vs (5 + 0 + 2) = 5 vs 7 → defender wins
	if result.AttackerWon {
		t.Errorf("expected defender wins: ignoresFortress skips terrain but fortification (+2) still applies")
	}
}

// Case 5: Leadership bonus applied correctly to co-located allies
func TestCombat_LeadershipBonus(t *testing.T) {
	// Aragorn(5, leader+1) + Gimli(3) attack → Gimli effective=4; 5+4=9 vs 5
	attackers := []game.AttackerUnit{
		{
			Snapshot: mkSnap("aragorn", "bree", 5, game.UnitActive),
			Config:   mkCfg("aragorn", game.SideFreePeoples, 5, true, 1, false, false, false, 0),
		},
		{
			Snapshot: mkSnap("gimli", "bree", 3, game.UnitActive),
			Config:   mkCfg("gimli", game.SideFreePeoples, 3, false, 0, false, false, false, 0),
		},
	}
	defenders := []game.DefenderUnit{{
		Snapshot: mkSnap("orc", "bree", 5, game.UnitActive),
		Config:   mkCfg("orc", game.SideShadow, 5, false, 0, false, false, false, 0),
	}}
	region := game.RegionState{ControlledBy: game.SideShadow}
	regionCfg := game.RegionConfig{Terrain: game.TerrainPlains}

	result := game.ResolveCombat(attackers, defenders, region, regionCfg)
	// 5 + (3+1) = 9 vs 5 → attacker wins, damage = 4
	if !result.AttackerWon {
		t.Errorf("expected attacker wins (9 vs 5), got defender holds")
	}
	if result.Damage != 4 {
		t.Errorf("expected damage=4, got %d", result.Damage)
	}
}

// Case 6: Indestructible unit: strength floors at 1
func TestCombat_IndestructibleUnit(t *testing.T) {
	snap := mkSnap("witch-king", "bree", 5, game.UnitActive)
	cfg := mkCfg("witch-king", game.SideShadow, 5, false, 0, false, true, false, 0)

	updated := game.ApplyDamageToUnit(snap, cfg, 10)
	if updated.Strength != 1 {
		t.Errorf("expected indestructible unit strength=1, got %d", updated.Strength)
	}
	if updated.Status != game.UnitActive {
		t.Errorf("expected indestructible unit status=ACTIVE, got %s", updated.Status)
	}
}
