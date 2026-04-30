package game

// DetectionResult holds results from the turn-end detection check.
type DetectionResult struct {
	Exposed            bool
	DetectedByUnitID   string
	DetectedInRegion   string
}

// RunDetection executes the detection formula from Section 3.6.
// Returns DetectionResult. Suppressed when turn <= hiddenUntilTurn.
// Behavior is entirely config-driven — no unit ID literals.
func RunDetection(
	turn int,
	hiddenUntilTurn int,
	ringBearerRegion string,
	units []UnitSnapshot,
	unitConfigs map[string]UnitConfig,
	sauronActiveInMordor bool,
	graph *Graph,
) DetectionResult {
	if turn <= hiddenUntilTurn {
		return DetectionResult{}
	}

	for _, u := range units {
		cfg, ok := unitConfigs[u.ID]
		if !ok || u.Status != UnitActive {
			continue
		}
		if cfg.Class != ClassNazgul {
			continue
		}

		effectiveRange := cfg.DetectionRange
		if sauronActiveInMordor {
			effectiveRange++
		}

		dist := graph.Distance(u.Region, ringBearerRegion)
		if dist >= 0 && dist <= effectiveRange {
			return DetectionResult{
				Exposed:          true,
				DetectedByUnitID: u.ID,
				DetectedInRegion: ringBearerRegion,
			}
		}
	}
	return DetectionResult{}
}

// IsSauronActiveInMordor checks if Sauron (any Maia unit with indestructible on shadow side in mordor)
// is active — driven entirely by config, no unit ID literals.
func IsSauronActiveInMordor(units []UnitSnapshot, unitConfigs map[string]UnitConfig) bool {
	for _, u := range units {
		cfg, ok := unitConfigs[u.ID]
		if !ok {
			continue
		}
		// Sauron is identified by: Maia, Shadow side, Indestructible, no MaiaAbilityPaths (empty)
		// and start region mordor — but we check current region
		if cfg.Maia && cfg.Side == SideShadow && cfg.Indestructible &&
			len(cfg.MaiaAbilityPaths) == 0 && u.Region == "mordor" &&
			u.Status == UnitActive {
			return true
		}
	}
	return false
}
