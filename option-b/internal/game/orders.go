package game

import "fmt"

// ValidationContext holds the KTable state needed to validate an order.
type ValidationContext struct {
	CurrentTurn int
	Units       map[string]UnitSnapshot
	UnitConfigs map[string]UnitConfig
	Paths       map[string]PathState
	PathConfigs map[string]PathConfig
	Regions     map[string]RegionState
	Graph       *Graph
	// PlayerSides maps playerId -> Side
	PlayerSides map[string]Side
	// OrdersThisTurn tracks unitIds that already have an order this turn
	OrdersThisTurn map[string]bool
	// SarumanDisabled is set permanently when Isengard falls
	SarumanDisabled bool
}

// ValidationError wraps an error code with a message.
type ValidationError struct {
	Code    ErrorCode
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidateOrder runs all 8 validation rules from Section 11.
func ValidateOrder(order Order, ctx ValidationContext) error {
	// Rule 8: duplicate unit order
	if ctx.OrdersThisTurn[order.UnitID] {
		return &ValidationError{Code: ErrDuplicateUnitOrder, Message: "unit already has an order this turn"}
	}

	// Rule 1: wrong turn
	if order.Turn != ctx.CurrentTurn {
		return &ValidationError{Code: ErrWrongTurn, Message: fmt.Sprintf("expected turn %d got %d", ctx.CurrentTurn, order.Turn)}
	}

	cfg, exists := ctx.UnitConfigs[order.UnitID]
	if !exists {
		return &ValidationError{Code: ErrNotYourUnit, Message: "unknown unit"}
	}

	// Rule 2: unit does not belong to player's side
	playerSide := ctx.PlayerSides[order.PlayerID]
	if cfg.Side != playerSide {
		return &ValidationError{Code: ErrNotYourUnit, Message: "unit belongs to the other side"}
	}

	snap, _ := ctx.Units[order.UnitID]

	switch order.OrderType {
	case OrderAssignRoute, OrderRedirectUnit:
		return validateRoute(order, snap, cfg, ctx)

	case OrderBlockPath, OrderSearchPath:
		// Rule 5: unit not in endpoint region
		return validatePathEndpointOrder(order, snap, ctx)

	case OrderAttackRegion:
		// Rule 6: target not adjacent or not enemy-controlled
		return validateAttackRegion(order, snap, cfg, ctx)

	case OrderMaiaAbility:
		// Rule 7: cooldown > 0
		return validateMaiaAbility(order, snap, cfg, ctx)

	case OrderDestroyRing:
		return validateDestroyRing(order, snap, cfg, ctx)

	case OrderFortifyRegion:
		if !cfg.CanFortify {
			return &ValidationError{Code: ErrInvalidTarget, Message: "unit cannot fortify"}
		}

	case OrderReinforceRegion, OrderDeployNazgul:
		// Basic adjacency check
		targetRegion := order.TargetRegion
		neighbors := ctx.Graph.Neighbors(snap.Region)
		adjacent := false
		for _, n := range neighbors {
			if n == targetRegion {
				adjacent = true
				break
			}
		}
		if !adjacent && snap.Region != targetRegion {
			return &ValidationError{Code: ErrUnitNotAdjacent, Message: "target not adjacent"}
		}
	}

	return nil
}

func validateRoute(order Order, snap UnitSnapshot, cfg UnitConfig, ctx ValidationContext) error {
	paths := order.PathIDs
	if order.OrderType == OrderRedirectUnit {
		paths = order.NewPathIDs
	}
	// Rule 3 & 4: for ring bearer, check first path
	if cfg.Class == ClassRingBearer && len(paths) > 0 {
		firstPath := paths[0]
		pathState, ok := ctx.Paths[firstPath]
		if !ok {
			return &ValidationError{Code: ErrInvalidPath, Message: "unknown path " + firstPath}
		}
		if pathState.Status == StatusBlocked {
			return &ValidationError{Code: ErrPathBlocked, Message: "path is blocked"}
		}
	}
	return nil
}

func validatePathEndpointOrder(order Order, snap UnitSnapshot, ctx ValidationContext) error {
	pathID := order.PathID
	if pathID == "" {
		return &ValidationError{Code: ErrInvalidPath, Message: "no path specified"}
	}
	pathCfg, ok := ctx.PathConfigs[pathID]
	if !ok {
		return &ValidationError{Code: ErrInvalidPath, Message: "unknown path"}
	}
	if snap.Region != pathCfg.From && snap.Region != pathCfg.To {
		return &ValidationError{Code: ErrUnitNotAdjacent, Message: "unit not at path endpoint"}
	}
	return nil
}

func validateAttackRegion(order Order, snap UnitSnapshot, cfg UnitConfig, ctx ValidationContext) error {
	target := order.TargetRegion
	neighbors := ctx.Graph.Neighbors(snap.Region)
	adjacent := false
	for _, n := range neighbors {
		if n == target {
			adjacent = true
			break
		}
	}
	if !adjacent {
		return &ValidationError{Code: ErrInvalidTarget, Message: "target not adjacent"}
	}
	targetRegion, ok := ctx.Regions[target]
	if !ok {
		return &ValidationError{Code: ErrInvalidTarget, Message: "unknown region"}
	}
	if targetRegion.ControlledBy == cfg.Side {
		return &ValidationError{Code: ErrInvalidTarget, Message: "target is friendly"}
	}
	return nil
}

func validateMaiaAbility(order Order, snap UnitSnapshot, cfg UnitConfig, ctx ValidationContext) error {
	if !cfg.Maia {
		return &ValidationError{Code: ErrInvalidTarget, Message: "unit is not Maia"}
	}
	if snap.Cooldown > 0 {
		return &ValidationError{Code: ErrAbilityOnCooldown, Message: fmt.Sprintf("cooldown %d turns remaining", snap.Cooldown)}
	}
	// Check if Saruman-type Maia is disabled
	if ctx.SarumanDisabled && len(cfg.MaiaAbilityPaths) > 0 {
		return &ValidationError{Code: ErrMaiaDisabled, Message: "Saruman is disabled"}
	}
	return nil
}

func validateDestroyRing(order Order, snap UnitSnapshot, cfg UnitConfig, ctx ValidationContext) error {
	if cfg.Class != ClassRingBearer {
		return &ValidationError{Code: ErrDestroyConditionNotMet, Message: "only ring bearer can destroy the ring"}
	}
	// Additional conditions checked at turn resolution time
	return nil
}

// AvailableOrders returns legal order types for a given unit and player.
func AvailableOrders(unitID, playerID string, ctx ValidationContext) []OrderType {
	cfg, ok := ctx.UnitConfigs[unitID]
	if !ok {
		return nil
	}
	playerSide := ctx.PlayerSides[playerID]
	if cfg.Side != playerSide {
		return nil
	}
	snap, _ := ctx.Units[unitID]
	if snap.Status != UnitActive {
		return nil
	}

	orders := []OrderType{OrderAssignRoute, OrderRedirectUnit}

	if cfg.CanFortify {
		orders = append(orders, OrderFortifyRegion)
	}
	if cfg.Maia && snap.Cooldown == 0 {
		orders = append(orders, OrderMaiaAbility)
	}
	if cfg.Class == ClassRingBearer {
		orders = append(orders, OrderDestroyRing)
	}
	if cfg.Side == SideShadow {
		orders = append(orders, OrderBlockPath, OrderSearchPath)
		if cfg.Class == ClassNazgul {
			orders = append(orders, OrderDeployNazgul)
		}
	}
	orders = append(orders, OrderAttackRegion, OrderReinforceRegion)

	return orders
}
