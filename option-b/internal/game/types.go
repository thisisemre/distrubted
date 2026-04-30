package game

// Side represents which faction a unit belongs to.
type Side string

const (
	SideFreePeoples Side = "FREE_PEOPLES"
	SideShadow      Side = "SHADOW"
	SideNeutral     Side = "NEUTRAL"
)

// UnitClass enumerates the six unit classes.
type UnitClass string

const (
	ClassRingBearer      UnitClass = "RingBearer"
	ClassFellowshipGuard UnitClass = "FellowshipGuard"
	ClassGondorArmy      UnitClass = "GondorArmy"
	ClassNazgul          UnitClass = "Nazgul"
	ClassUrukHaiLegion   UnitClass = "UrukHaiLegion"
	ClassMaia            UnitClass = "Maia"
)

// Terrain types for regions.
type Terrain string

const (
	TerrainPlains   Terrain = "PLAINS"
	TerrainMountains Terrain = "MOUNTAINS"
	TerrainForest   Terrain = "FOREST"
	TerrainFortress Terrain = "FORTRESS"
	TerrainVolcanic Terrain = "VOLCANIC"
	TerrainSwamp    Terrain = "SWAMP"
)

// SpecialRole of a region.
type SpecialRole string

const (
	RoleNone               SpecialRole = "NONE"
	RoleRingBearerStart    SpecialRole = "RING_BEARER_START"
	RoleRingDestructionSite SpecialRole = "RING_DESTRUCTION_SITE"
	RoleShadowStronghold   SpecialRole = "SHADOW_STRONGHOLD"
)

// PathStatus tracks the traversal state of a path.
type PathStatus string

const (
	StatusOpen            PathStatus = "OPEN"
	StatusThreatened      PathStatus = "THREATENED"
	StatusBlocked         PathStatus = "BLOCKED"
	StatusTemporarilyOpen PathStatus = "TEMPORARILY_OPEN"
)

// UnitStatus is the lifecycle state of a unit.
type UnitStatus string

const (
	UnitActive    UnitStatus = "ACTIVE"
	UnitDestroyed UnitStatus = "DESTROYED"
	UnitRespawning UnitStatus = "RESPAWNING"
)

// UnitConfig holds static configuration loaded from units.conf.
// No unit ID string literals appear in game logic — behavior is driven by these fields.
type UnitConfig struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Class            UnitClass `json:"class"`
	Side             Side      `json:"side"`
	StartRegion      string    `json:"start"`
	Strength         int       `json:"strength"`
	Leadership       bool      `json:"leadership"`
	LeadershipBonus  int       `json:"leadershipBonus"`
	Indestructible   bool      `json:"indestructible"`
	DetectionRange   int       `json:"detectionRange"`
	Respawns         bool      `json:"respawns"`
	RespawnTurns     int       `json:"respawnTurns"`
	Maia             bool      `json:"maia"`
	MaiaAbilityPaths []string  `json:"maiaAbilityPaths"`
	IgnoresFortress  bool      `json:"ignoresFortress"`
	CanFortify       bool      `json:"canFortify"`
	Cooldown         int       `json:"cooldown"`
}

// RegionConfig holds static region configuration from map.conf.
type RegionConfig struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Terrain      Terrain     `json:"terrain"`
	SpecialRole  SpecialRole `json:"specialRole"`
	StartControl Side        `json:"startControl"`
	StartThreat  int         `json:"startThreat"`
}

// PathConfig holds static path configuration from map.conf.
type PathConfig struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Cost int    `json:"cost"`
}

// UnitSnapshot is the mutable runtime state of a unit (lives in KTable).
type UnitSnapshot struct {
	ID           string     `json:"id"`
	Region       string     `json:"region"`   // always "" for ring-bearer in public state
	Strength     int        `json:"strength"`
	Status       UnitStatus `json:"status"`
	RespawnTurns int        `json:"respawnTurns"`
	Route        []string   `json:"route"`
	RouteIdx     int        `json:"routeIdx"`
	Cooldown     int        `json:"cooldown"`
}

// RegionState is the mutable runtime state of a region.
type RegionState struct {
	ID           string `json:"id"`
	ControlledBy Side   `json:"controlledBy"`
	ThreatLevel  int    `json:"threatLevel"`
	Fortified    bool   `json:"fortified"`
	FortifyTurns int    `json:"fortifyTurns"`
	UnitsPresent []string `json:"unitsPresent"`
}

// PathState is the mutable runtime state of a path.
type PathState struct {
	ID               string     `json:"id"`
	Status           PathStatus `json:"status"`
	SurveillanceLevel int       `json:"surveillanceLevel"`
	TempOpenTurns    int        `json:"tempOpenTurns"`
	BlockedBy        string     `json:"blockedBy"` // unit id or ""
}

// RingBearerState holds the secret state of the ring bearer (never exposed publicly).
type RingBearerState struct {
	TrueRegion         string `json:"trueRegion"`
	Exposed            bool   `json:"exposed"`
	Route              []string `json:"route"`
	RouteIdx           int    `json:"routeIdx"`
	LastDetectedTurn   int    `json:"lastDetectedTurn"`
	LastDetectedRegion string `json:"lastDetectedRegion"`
}

// GameConfig holds global game settings.
type GameConfig struct {
	HiddenUntilTurn    int `json:"hiddenUntilTurn"`
	MaxTurns           int `json:"maxTurns"`
	TurnDurationSeconds int `json:"turnDurationSeconds"`
}

// OrderType enumerates all valid player orders.
type OrderType string

const (
	OrderAssignRoute    OrderType = "ASSIGN_ROUTE"
	OrderRedirectUnit   OrderType = "REDIRECT_UNIT"
	OrderDestroyRing    OrderType = "DESTROY_RING"
	OrderMaiaAbility    OrderType = "MAIA_ABILITY"
	OrderBlockPath      OrderType = "BLOCK_PATH"
	OrderSearchPath     OrderType = "SEARCH_PATH"
	OrderAttackRegion   OrderType = "ATTACK_REGION"
	OrderReinforceRegion OrderType = "REINFORCE_REGION"
	OrderFortifyRegion  OrderType = "FORTIFY_REGION"
	OrderDeployNazgul   OrderType = "DEPLOY_NAZGUL"
)

// ErrorCode enumerates validation error codes.
type ErrorCode string

const (
	ErrWrongTurn              ErrorCode = "WRONG_TURN"
	ErrNotYourUnit            ErrorCode = "NOT_YOUR_UNIT"
	ErrInvalidPath            ErrorCode = "INVALID_PATH"
	ErrPathBlocked            ErrorCode = "PATH_BLOCKED"
	ErrUnitNotAdjacent        ErrorCode = "UNIT_NOT_ADJACENT"
	ErrInvalidTarget          ErrorCode = "INVALID_TARGET"
	ErrDuplicateUnitOrder     ErrorCode = "DUPLICATE_UNIT_ORDER"
	ErrAbilityOnCooldown      ErrorCode = "ABILITY_ON_COOLDOWN"
	ErrMaiaDisabled           ErrorCode = "MAIA_DISABLED"
	ErrDestroyConditionNotMet ErrorCode = "DESTROY_CONDITION_NOT_MET"
)

// Order is a player order submitted via the API.
type Order struct {
	OrderType    OrderType `json:"orderType"`
	PlayerID     string    `json:"playerId"`
	UnitID       string    `json:"unitId"`
	Turn         int       `json:"turn"`
	PathIDs      []string  `json:"pathIds,omitempty"`
	NewPathIDs   []string  `json:"newPathIds,omitempty"`
	TargetPathID string    `json:"targetPathId,omitempty"`
	PathID       string    `json:"pathId,omitempty"`
	TargetRegion string    `json:"targetRegion,omitempty"`
}

// ValidatedOrder is an order that passed validation, optionally enriched.
type ValidatedOrder struct {
	Order
	RouteRiskScore  *int     `json:"routeRiskScore,omitempty"`
	ThreatenedPaths []string `json:"threatenedPaths,omitempty"`
	BlockedPaths    []string `json:"blockedPaths,omitempty"`
}

// GameEvent is an event produced to Kafka topics.
type GameEvent struct {
	Type      string      `json:"type"`
	Topic     string      `json:"topic"`
	Key       string      `json:"key"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

// WorldStateSnapshot is broadcast each turn end.
type WorldStateSnapshot struct {
	Turn    int            `json:"turn"`
	Regions []RegionState  `json:"regions"`
	Units   []UnitSnapshot `json:"units"`
	Timestamp int64        `json:"timestamp"`
}

// PlayerID constants for the two sides.
const (
	PlayerLightSide = "light"
	PlayerDarkSide  = "dark"
)
