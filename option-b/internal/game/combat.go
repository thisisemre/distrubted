package game

// CombatResult holds the outcome of a battle.
type CombatResult struct {
	AttackerWon    bool
	Damage         int
	RegionControl  Side
}

// ResolveCombat resolves a battle between attackers and defenders in the target region.
// All behavior is driven by UnitConfig fields — no unit ID literals.
func ResolveCombat(
	attackers []AttackerUnit,
	defenders []DefenderUnit,
	region RegionState,
	regionCfg RegionConfig,
) CombatResult {
	attackerPower := computeAttackerPower(attackers)
	defenderPower := computeDefenderPower(defenders, region, regionCfg, hasIgnoresFortress(attackers))

	if attackerPower > defenderPower {
		damage := attackerPower - defenderPower
		return CombatResult{
			AttackerWon:   true,
			Damage:        damage,
			RegionControl: attackerSide(attackers),
		}
	}
	// Defenders hold — each attacker loses 1 strength
	return CombatResult{
		AttackerWon:   false,
		Damage:        1,
		RegionControl: region.ControlledBy,
	}
}

// AttackerUnit pairs a unit snapshot with its config.
type AttackerUnit struct {
	Snapshot UnitSnapshot
	Config   UnitConfig
}

// DefenderUnit pairs a defender snapshot with its config.
type DefenderUnit struct {
	Snapshot UnitSnapshot
	Config   UnitConfig
}

func computeAttackerPower(attackers []AttackerUnit) int {
	power := 0
	leaderBonus := leaderBonusFromGroup(attackers)
	for _, a := range attackers {
		eff := a.Snapshot.Strength
		if !a.Config.Leadership {
			eff += leaderBonus
		}
		power += eff
	}
	return power
}

func computeDefenderPower(defenders []DefenderUnit, region RegionState, regionCfg RegionConfig, ignoreFortress bool) int {
	power := 0
	leaderBonus := defenderLeaderBonus(defenders)
	for _, d := range defenders {
		eff := d.Snapshot.Strength
		if !d.Config.Leadership {
			eff += leaderBonus
		}
		power += eff
	}

	if !ignoreFortress {
		power += terrainBonus(regionCfg.Terrain)
	}
	if region.Fortified {
		power += 2
	}
	return power
}

func terrainBonus(t Terrain) int {
	switch t {
	case TerrainFortress:
		return 2
	case TerrainMountains:
		return 1
	default:
		return 0
	}
}

func hasIgnoresFortress(attackers []AttackerUnit) bool {
	for _, a := range attackers {
		if a.Config.IgnoresFortress {
			return true
		}
	}
	return false
}

func leaderBonusFromGroup(units []AttackerUnit) int {
	for _, u := range units {
		if u.Config.Leadership {
			return u.Config.LeadershipBonus
		}
	}
	return 0
}

func defenderLeaderBonus(units []DefenderUnit) int {
	for _, u := range units {
		if u.Config.Leadership {
			return u.Config.LeadershipBonus
		}
	}
	return 0
}

func attackerSide(attackers []AttackerUnit) Side {
	if len(attackers) > 0 {
		return attackers[0].Config.Side
	}
	return SideNeutral
}

// ApplyDamageToUnit applies damage to a unit, respecting indestructible and respawn config.
func ApplyDamageToUnit(snap UnitSnapshot, cfg UnitConfig, damage int) UnitSnapshot {
	raw := snap.Strength - damage
	if cfg.Indestructible {
		newStr := raw
		if newStr < 1 {
			newStr = 1
		}
		snap.Strength = newStr
		return snap
	}
	if raw <= 0 {
		if cfg.Respawns {
			snap.Strength = 0
			snap.Status = UnitRespawning
			snap.RespawnTurns = cfg.RespawnTurns
			snap.Region = ""
		} else {
			snap.Strength = 0
			snap.Status = UnitDestroyed
		}
	} else {
		snap.Strength = raw
	}
	return snap
}
