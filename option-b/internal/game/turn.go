package game

import "time"

// TurnResult holds all events produced by processing one turn.
type TurnResult struct {
	Turn           int
	Events         []GameEvent
	Winner         Side   // "" if no winner yet
	WinCause       string
	Draw           bool
	Snapshot       WorldStateSnapshot
	RingBearerMoved bool
	RingBearerExposed bool
	DetectionResult DetectionResult
}

// TurnState is the complete mutable game state for one turn.
type TurnState struct {
	Turn        int
	Units       map[string]UnitSnapshot
	UnitConfigs map[string]UnitConfig
	Paths       map[string]PathState
	PathConfigs map[string]PathConfig
	Regions     map[string]RegionState
	RegionConfigs map[string]RegionConfig
	RingBearer  RingBearerState
	Graph       *Graph
	GameCfg     GameConfig
	SarumanDisabled bool
}

// ProcessTurn executes the 13-step turn processing pipeline.
func ProcessTurn(state TurnState, orders []ValidatedOrder) TurnResult {
	result := TurnResult{Turn: state.Turn}
	now := time.Now().UnixMilli()

	// Step 1: orders already collected — proceed.

	// Step 2: process AssignRoute and RedirectUnit
	for _, o := range orders {
		switch o.OrderType {
		case OrderAssignRoute:
			if u, ok := state.Units[o.UnitID]; ok {
				if state.UnitConfigs[o.UnitID].Class == ClassRingBearer {
					state.RingBearer.Route = o.PathIDs
					state.RingBearer.RouteIdx = 0
				} else {
					u.Route = o.PathIDs
					u.RouteIdx = 0
					state.Units[o.UnitID] = u
				}
			}
		case OrderRedirectUnit:
			if u, ok := state.Units[o.UnitID]; ok {
				if state.UnitConfigs[o.UnitID].Class == ClassRingBearer {
					state.RingBearer.Route = o.NewPathIDs
					state.RingBearer.RouteIdx = 0
				} else {
					u.Route = o.NewPathIDs
					u.RouteIdx = 0
					state.Units[o.UnitID] = u
				}
			}
		}
	}

	// Step 3: BlockPath and SearchPath
	for _, o := range orders {
		switch o.OrderType {
		case OrderBlockPath:
			if p, ok := state.Paths[o.PathID]; ok {
				p.Status = StatusBlocked
				p.BlockedBy = o.UnitID
				state.Paths[o.PathID] = p
				result.Events = append(result.Events, GameEvent{
					Type: "PathStatusChanged", Topic: "game.events.path",
					Key: o.PathID, Payload: p, Timestamp: now,
				})
			}
		case OrderSearchPath:
			if p, ok := state.Paths[o.PathID]; ok {
				if p.SurveillanceLevel < 3 {
					p.SurveillanceLevel++
				}
				state.Paths[o.PathID] = p
			}
		}
	}

	// Revert BLOCKED paths whose blocking unit has moved away
	for pid, p := range state.Paths {
		if p.Status == StatusBlocked && p.BlockedBy != "" {
			blocker, ok := state.Units[p.BlockedBy]
			pathCfg := state.PathConfigs[pid]
			if !ok || (blocker.Region != pathCfg.From && blocker.Region != pathCfg.To) {
				p.Status = StatusOpen
				p.BlockedBy = ""
				state.Paths[pid] = p
			}
		}
	}

	// Step 4: ReinforceRegion and DeployNazgul
	for _, o := range orders {
		switch o.OrderType {
		case OrderReinforceRegion:
			moveUnitToRegion(o.UnitID, o.TargetRegion, state)
		case OrderDeployNazgul:
			moveUnitToRegion(o.UnitID, o.TargetRegion, state)
		}
	}

	// Step 5: FortifyRegion
	for _, o := range orders {
		if o.OrderType == OrderFortifyRegion {
			if u, ok := state.Units[o.UnitID]; ok {
				if r, ok2 := state.Regions[u.Region]; ok2 {
					r.Fortified = true
					r.FortifyTurns = 2
					state.Regions[u.Region] = r
				}
			}
		}
	}

	// Step 6: MaiaAbility
	for _, o := range orders {
		if o.OrderType == OrderMaiaAbility {
			cfg := state.UnitConfigs[o.UnitID]
			u := state.Units[o.UnitID]

			if len(cfg.MaiaAbilityPaths) == 0 {
				// Gandalf-type: OpenPath
				if p, ok := state.Paths[o.TargetPathID]; ok && p.Status == StatusBlocked {
					p.Status = StatusTemporarilyOpen
					p.TempOpenTurns = 2
					state.Paths[o.TargetPathID] = p
					result.Events = append(result.Events, GameEvent{
						Type: "PathStatusChanged", Topic: "game.events.path",
						Key: o.TargetPathID, Payload: p, Timestamp: now,
					})
				}
			} else {
				// Saruman-type: CorruptPath
				if !state.SarumanDisabled {
					if p, ok := state.Paths[o.TargetPathID]; ok {
						p.SurveillanceLevel = 3
						state.Paths[o.TargetPathID] = p
						result.Events = append(result.Events, GameEvent{
							Type: "PathCorrupted", Topic: "game.events.path",
							Key: o.TargetPathID, Payload: p, Timestamp: now,
						})
					}
				}
			}
			// Set cooldown
			u.Cooldown = cfg.Cooldown
			state.Units[o.UnitID] = u
		}
	}

	// Step 7: Auto-advance all units with routes
	for uid, u := range state.Units {
		cfg := state.UnitConfigs[uid]
		if cfg.Class == ClassRingBearer {
			continue // handled separately below
		}
		if u.Status != UnitActive || len(u.Route) == 0 || u.RouteIdx >= len(u.Route) {
			continue
		}
		nextPathID := u.Route[u.RouteIdx]
		p := state.Paths[nextPathID]
		if p.Status == StatusBlocked {
			result.Events = append(result.Events, GameEvent{
				Type: "RouteBlocked", Topic: "game.events.unit",
				Key: uid, Payload: map[string]interface{}{"unitId": uid, "pathId": nextPathID}, Timestamp: now,
			})
			continue
		}
		// Advance
		pathCfg := state.PathConfigs[nextPathID]
		dest := pathCfg.To
		if pathCfg.From == u.Region {
			dest = pathCfg.To
		} else {
			dest = pathCfg.From
		}
		fromRegion := u.Region
		u.Region = dest
		u.RouteIdx++
		state.Units[uid] = u
		result.Events = append(result.Events, GameEvent{
			Type: "UnitMoved", Topic: "game.events.unit",
			Key: uid, Payload: map[string]interface{}{"unitId": uid, "from": fromRegion, "to": dest, "turn": state.Turn}, Timestamp: now,
		})
		if u.RouteIdx >= len(u.Route) {
			result.Events = append(result.Events, GameEvent{
				Type: "RouteComplete", Topic: "game.events.unit",
				Key: uid, Payload: map[string]interface{}{"unitId": uid}, Timestamp: now,
			})
		}
	}
	// Auto-advance Ring Bearer
	rb := state.RingBearer
	if len(rb.Route) > 0 && rb.RouteIdx < len(rb.Route) {
		nextPathID := rb.Route[rb.RouteIdx]
		p := state.Paths[nextPathID]
		if p.Status == StatusBlocked {
			result.Events = append(result.Events, GameEvent{
				Type: "RouteBlocked", Topic: "game.events.unit",
				Key: "ring-bearer", Payload: map[string]interface{}{"unitId": "ring-bearer", "pathId": nextPathID}, Timestamp: now,
			})
		} else {
			pathCfg := state.PathConfigs[nextPathID]
			dest := pathCfg.To
			if pathCfg.To == rb.TrueRegion {
				dest = pathCfg.From
			}
			rb.TrueRegion = dest
			rb.RouteIdx++
			result.RingBearerMoved = true
			// Emit to ring.position (Light Side only)
			result.Events = append(result.Events, GameEvent{
				Type: "RingBearerMoved", Topic: "game.ring.position",
				Key: "ring-bearer", Payload: map[string]interface{}{"trueRegion": dest, "turn": state.Turn}, Timestamp: now,
			})
			// Check surveillance exposure
			if p.SurveillanceLevel >= 1 && state.Turn > state.GameCfg.HiddenUntilTurn {
				rb.Exposed = true
				result.Events = append(result.Events, GameEvent{
					Type: "RingBearerSpotted", Topic: "game.ring.detection",
					Key: "ring-bearer", Payload: map[string]interface{}{"pathId": nextPathID, "turn": state.Turn}, Timestamp: now,
				})
			}
		}
	}
	state.RingBearer = rb

	// Step 8: AttackRegion
	for _, o := range orders {
		if o.OrderType != OrderAttackRegion {
			continue
		}
		target := o.TargetRegion
		attackerCfg := state.UnitConfigs[o.UnitID]

		// Collect all co-located attackers
		attackers := collectAttackers(o.UnitID, target, state)
		defenders := collectDefenders(target, attackerCfg.Side, state)

		if len(defenders) == 0 {
			// Uncontested — take region
			if r, ok := state.Regions[target]; ok {
				r.ControlledBy = attackerCfg.Side
				state.Regions[target] = r
			}
			continue
		}

		regionCfg := state.RegionConfigs[target]
		combatResult := ResolveCombat(attackers, defenders, state.Regions[target], regionCfg)

		result.Events = append(result.Events, GameEvent{
			Type: "BattleResolved", Topic: "game.events.region",
			Key: target, Payload: map[string]interface{}{
				"regionId": target, "attackerWon": combatResult.AttackerWon,
				"turn": state.Turn,
			}, Timestamp: now,
		})

		if combatResult.AttackerWon {
			if r, ok := state.Regions[target]; ok {
				r.ControlledBy = combatResult.RegionControl
				state.Regions[target] = r
			}
			// Apply damage to defenders
			for _, d := range defenders {
				updated := ApplyDamageToUnit(d.Snapshot, d.Config, combatResult.Damage)
				state.Units[d.Snapshot.ID] = updated
			}
			// Check Isengard destruction
			if target == "isengard" && combatResult.RegionControl == SideFreePeoples {
				state.SarumanDisabled = true
				result.Events = append(result.Events, GameEvent{
					Type: "IsengardDestroyed", Topic: "game.events.region",
					Key: target, Payload: map[string]interface{}{"turn": state.Turn}, Timestamp: now,
				})
			}
		} else {
			// Attackers repelled — each loses 1
			for _, a := range attackers {
				updated := ApplyDamageToUnit(a.Snapshot, a.Config, 1)
				state.Units[a.Snapshot.ID] = updated
			}
		}
	}

	// Step 9: Decrement TEMPORARILY_OPEN timers
	for pid, p := range state.Paths {
		if p.Status == StatusTemporarilyOpen {
			p.TempOpenTurns--
			if p.TempOpenTurns <= 0 {
				if p.BlockedBy != "" {
					blocker := state.Units[p.BlockedBy]
					pathCfg := state.PathConfigs[pid]
					if blocker.Region == pathCfg.From || blocker.Region == pathCfg.To {
						p.Status = StatusBlocked
					} else {
						p.Status = StatusOpen
						p.BlockedBy = ""
					}
				} else {
					p.Status = StatusOpen
				}
			}
			state.Paths[pid] = p
		}
	}

	// Step 10: Decrement fortification timers
	for rid, r := range state.Regions {
		if r.Fortified && r.FortifyTurns > 0 {
			r.FortifyTurns--
			if r.FortifyTurns <= 0 {
				r.Fortified = false
			}
			state.Regions[rid] = r
		}
	}

	// Step 11: Decrement respawn and cooldown counters
	for uid, u := range state.Units {
		cfg := state.UnitConfigs[uid]
		if u.Status == UnitRespawning {
			u.RespawnTurns--
			if u.RespawnTurns <= 0 {
				u.Status = UnitActive
				u.Region = cfg.StartRegion
				u.Strength = cfg.Strength
				u.RespawnTurns = 0
			}
			state.Units[uid] = u
		}
		if u.Cooldown > 0 {
			u.Cooldown--
			state.Units[uid] = u
		}
	}

	// Step 12: Detection check
	allUnits := unitMapToSlice(state.Units)
	sauronActive := IsSauronActiveInMordor(allUnits, state.UnitConfigs)
	det := RunDetection(state.Turn, state.GameCfg.HiddenUntilTurn,
		state.RingBearer.TrueRegion, allUnits, state.UnitConfigs, sauronActive, state.Graph)
	result.DetectionResult = det
	if det.Exposed {
		state.RingBearer.Exposed = true
		result.RingBearerExposed = true
		result.Events = append(result.Events, GameEvent{
			Type: "RingBearerDetected", Topic: "game.ring.detection",
			Key: "dark", Payload: map[string]interface{}{
				"regionId": det.DetectedInRegion, "turn": state.Turn,
			}, Timestamp: now,
		})
	}

	// Step 13: Win condition evaluation
	winner, cause := evaluateWinConditions(state)
	result.Winner = winner
	result.WinCause = cause
	if state.Turn >= state.GameCfg.MaxTurns && winner == "" {
		result.Draw = true
	}

	// Build snapshot (ring bearer region stripped for public)
	units := unitMapToSlice(state.Units)
	for i, u := range units {
		if state.UnitConfigs[u.ID].Class == ClassRingBearer {
			units[i].Region = ""
		}
	}
	regions := regionMapToSlice(state.Regions)
	result.Snapshot = WorldStateSnapshot{
		Turn:      state.Turn,
		Units:     units,
		Regions:   regions,
		Timestamp: now,
	}

	// Reset exposure
	rb2 := state.RingBearer
	rb2.Exposed = false
	state.RingBearer = rb2

	return result
}

func evaluateWinConditions(state TurnState) (Side, string) {
	rb := state.RingBearer

	// Dark Side wins: Nazgul in same region as Ring Bearer AND exposed
	if rb.Exposed {
		for _, u := range state.Units {
			cfg := state.UnitConfigs[u.ID]
			if cfg.Class == ClassNazgul && u.Status == UnitActive && u.Region == rb.TrueRegion {
				return SideShadow, "RING_BEARER_CAUGHT"
			}
		}
	}

	// Light Side wins: Ring Bearer at mount-doom + DestroyRing order + no Dark Side unit there
	if rb.TrueRegion == "mount-doom" {
		darkUnitPresent := false
		for _, u := range state.Units {
			cfg := state.UnitConfigs[u.ID]
			if cfg.Side == SideShadow && u.Region == "mount-doom" && u.Status == UnitActive {
				darkUnitPresent = true
				break
			}
		}
		if !darkUnitPresent {
			return SideFreePeoples, "RING_DESTROYED"
		}
	}

	return "", ""
}

func collectAttackers(initiatorID, targetRegion string, state TurnState) []AttackerUnit {
	initiatorCfg := state.UnitConfigs[initiatorID]
	result := []AttackerUnit{}
	for _, u := range state.Units {
		cfg := state.UnitConfigs[u.ID]
		if cfg.Side == initiatorCfg.Side && u.Status == UnitActive {
			// Units in same region as initiator (or initiator itself) attack together
			if u.Region == state.Units[initiatorID].Region {
				result = append(result, AttackerUnit{Snapshot: u, Config: cfg})
			}
		}
	}
	return result
}

func collectDefenders(targetRegion string, attackerSide Side, state TurnState) []DefenderUnit {
	result := []DefenderUnit{}
	for _, u := range state.Units {
		cfg := state.UnitConfigs[u.ID]
		if cfg.Side != attackerSide && u.Status == UnitActive && u.Region == targetRegion {
			result = append(result, DefenderUnit{Snapshot: u, Config: cfg})
		}
	}
	return result
}

func moveUnitToRegion(unitID, targetRegion string, state TurnState) {
	if u, ok := state.Units[unitID]; ok {
		u.Region = targetRegion
		state.Units[unitID] = u
	}
}

func unitMapToSlice(m map[string]UnitSnapshot) []UnitSnapshot {
	result := make([]UnitSnapshot, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func regionMapToSlice(m map[string]RegionState) []RegionState {
	result := make([]RegionState, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
