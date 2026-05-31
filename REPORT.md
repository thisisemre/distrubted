# Ring of the Middle Earth — Project Report

**Course:** Distributed Systems  
**Technology:** Go (Option B)  
**Repository:** https://github.com/thisisemre/distrubted  

---

## 1. Project Overview

*Ring of the Middle Earth* is a distributed, browser-based, turn-based strategy game. Two players compete in separate browser tabs: the **Light Side** escorts the Ring Bearer from The Shire to Mount Doom, while the **Dark Side** hunts it down with Nazgûl units. The game demonstrates core distributed systems concepts including event streaming, information asymmetry, fault tolerance, and exactly-once semantics.

---

## 2. System Architecture

```
Browser A (Light Side)           Browser B (Dark Side)
  POST /order                      POST /order
  GET  /events (SSE)               GET  /events (SSE)
  GET  /analysis/routes            GET  /analysis/intercept
        |                                |
        +----------[ nginx ]-----------+
                       |
           +-----------+-----------+
           |   3 × Go instances    |
           |  go-1  go-2  go-3    |
           |  (stateless HTTP +   |
           |   SSE + game engine) |
           +-----------+-----------+
                       |
           +-----------+-----------+
           |   3-broker Kafka      |
           |   10 topics           |
           |   Schema Registry     |
           +-----------+-----------+
```

### Component Breakdown

| Component | Technology | Role |
|---|---|---|
| Load balancer | nginx 1.25 | Routes HTTP orders round-robin; SSE sticky by `$arg_playerId` |
| Application tier | Go 1.22 × 3 | Stateless HTTP, SSE, game engine, turn processor |
| Message broker | Apache Kafka 3-broker cluster | Event streaming, order routing, state replay |
| Schema registry | Confluent Schema Registry | Avro schema validation for 13 event types |
| Frontend | Vanilla JS + SVG | Interactive map, order form, live event log |

---

## 3. Go Instance Goroutine Architecture (Section 28)

Each Go instance runs the following concurrent goroutines:

| Goroutine | Channel | Role |
|---|---|---|
| `KafkaConsumer × 7` | → `eventCh` (cap 100) | One reader per subscribed topic |
| `EventRouter` | `eventCh` → SSE/`engineCh`/`cacheUpdateCh` | Single information-asymmetry enforcement point |
| `CacheManager` | `cacheUpdateCh` → `WorldStateCache` | Applies broadcast snapshots; preserves `RingBearerState` |
| `TurnProcessor` | `engineCh` + 30s ticker | Accumulates orders; runs 13-step pipeline |
| `Pipeline 1` (4 workers) | cap-20 buffer | Computes route risk scores for Light Side |
| `Pipeline 2` (4 workers) | cap-30 buffer | Computes Nazgûl interception plans for Dark Side |
| `SSE goroutines` | `lightSideSSECh` / `darkSideSSECh` | One per connected player |
| `HTTP server` | — | REST API on `:8080` |
| `Main select loop` | 7 cases | Orders, connections, analysis, signals, turn tick |

### Main Select Loop (7 Cases)

```go
select {
case raw   := <-rawOrderCh:       validator.Validate(raw)
case       <-newConnectionCh:     log.Println("New SSE connection")
case       <-disconnectCh:        log.Println("SSE disconnect")
case       <-analysisRequestCh:   log.Println("Analysis request")
case       <-cacheUpdateCh:       // prevents starvation; CacheManager owns actual update
case tick  := <-ticker.C:         log.Printf("Turn tick: %s", tick)
case sig   := <-signalCh:         cancel(); return
}
```

---

## 4. Kafka Topics (10 Total)

| Topic | Partitions | Retention | Key | Purpose |
|---|---|---|---|---|
| `game.orders.raw` | 3 | 1h | playerId | Raw player orders from HTTP |
| `game.orders.validated` | 6 | 1h | unitId | Orders passing validation |
| `game.events.unit` | 6 | 7d | unitId | Unit movements, combat results |
| `game.events.region` | 6 | 7d | regionId | Region control changes |
| `game.events.path` | 6 | 7d | pathId | Path status changes |
| `game.session` | 1 | compact | — | Game session metadata |
| `game.broadcast` | 1 | 1h | — | Full world snapshot per turn |
| `game.ring.position` | 1 | 1h | — | Ring Bearer true position (Light Side only) |
| `game.ring.detection` | 2 | 1h | playerId | Nazgûl detection events |
| `game.dlq` | 3 | 7d | errorCode | Dead-letter queue for failed orders |

---

## 5. Information Asymmetry (Demo Scenario 1)

`EventRouter` is the **single enforcement point** for information asymmetry. The architecture guarantees that the Dark Side can never learn the Ring Bearer's true position through three independent mechanisms:

1. **Topic routing**: `game.ring.position` events are only sent to `lightSideSSECh`. They are never forwarded to `darkSideSSECh`.

2. **Snapshot stripping**: Before broadcasting a world-state snapshot to the Dark Side SSE channel, `stripRingBearer()` sets `ring-bearer.Region = ""`.

3. **Public state API**: `GetPublicState("dark")` always returns `ringBearer.currentRegion = ""`, enforced in `WorldStateCache`.

```go
// EventRouter — single enforcement point
case "game.ring.position":
    r.lightSideSSECh <- event  // Light Side sees true position
    r.cacheUpdateCh  <- event
    // dark side channel intentionally omitted

case "game.broadcast":
    r.lightSideSSECh <- event
    r.darkSideSSECh  <- stripRingBearer(event)  // position stripped
```

**Verified**: `curl "http://localhost/game/state?playerId=dark" | jq '.ringBearer'` returns `{"currentRegion":"","lastDetectedRegion":"","lastDetectedTurn":0}` regardless of actual Ring Bearer location.

---

## 6. Turn Processing Pipeline (13 Steps)

`ProcessTurn(state TurnState, orders []ValidatedOrder) TurnResult` implements the full turn in a pure function (no side effects, no I/O):

| Step | Action |
|---|---|
| 1 | Collect pending orders (already accumulated in `TurnProcessor`) |
| 2 | Process `ASSIGN_ROUTE` and `REDIRECT_UNIT` orders |
| 3 | Process `BLOCK_PATH` and `SEARCH_PATH` orders |
| — | Revert blocked paths whose blocking unit has moved away |
| 4 | Process `REINFORCE_REGION` and `DEPLOY_NAZGUL` orders |
| 5 | Process `FORTIFY_REGION` orders |
| 6 | Process `MAIA_ABILITY` orders (Gandalf: open path; Saruman: corrupt path) |
| 7 | Auto-advance all units with active routes; auto-advance Ring Bearer |
| 8 | Resolve `ATTACK_REGION` combat (terrain modifiers, leadership bonuses) |
| 9 | Decrement `TEMPORARILY_OPEN` path timers |
| 10 | Decrement fortification timers |
| 11 | Decrement respawn and cooldown counters |
| 12 | Run detection check (Nazgûl proximity + surveillance level) |
| 13 | Evaluate win conditions; build and return `TurnResult` |

---

## 7. Order Validation (Kafka Streams Topology 1)

`OrderValidator` implements a KTable-based validation pipeline:

- Consumes from `rawOrderCh` (in-process channel backed by HTTP POST `/order`)
- Validates against 8 rules: turn match, unit ownership, path validity, blocked path, adjacency, enemy target, Maia cooldown, duplicate order
- Produces to `game.orders.validated` on pass, `game.dlq` on fail
- KTable is rebuilt from `WorldStateCache` after each turn via `validator.SetKTable(buildKTable(worldCache))`, advancing `CurrentTurn` so orders for the next turn are accepted

```go
// After each turn tick
validator.SetKTable(buildKTable(worldCache))
// Advances CurrentTurn, refreshes unit/path/region state, resets OrdersThisTurn
```

---

## 8. Maia Dispatch (Demo Scenario 2)

Gandalf and Saruman respond to the same `MAIA_ABILITY` order type. Dispatch is driven entirely by configuration — no unit ID string literals appear in the game logic:

```go
// In ProcessTurn — Step 6
cfg := state.UnitConfigs[o.UnitID]
if len(cfg.MaiaAbilityPaths) == 0 {
    // Gandalf-type: open a blocked path (TEMPORARILY_OPEN for 2 turns)
    p.Status = StatusTemporarilyOpen
} else {
    // Saruman-type: corrupt a path (SurveillanceLevel → 3)
    p.SurveillanceLevel = 3
}
```

`config/units.conf` sets `maiaAbilityPaths = []` for Gandalf and `maiaAbilityPaths = [...]` for Saruman. When Isengard falls to the Free Peoples, `state.SarumanDisabled = true` is set and Saruman's ability is permanently disabled.

---

## 9. Fault Tolerance (Demo Scenario 3)

The system tolerates Go instance failures through Kafka's consumer group protocol:

```bash
# Kill go-2
docker compose stop go-2
# Kafka reassigns go-2's partitions to go-1 and go-3
# Game continues uninterrupted on remaining two instances

# Restart go-2
docker compose start go-2
# go-2 rejoins consumer group, replays partition state from Kafka
# KTable rebuilt from latest offsets; in-memory cache restored
```

Each instance subscribes with its own consumer group ID (`game-engine-1`, `game-engine-2`, `game-engine-3`), receiving all events independently. State is not stored in Go instances — it lives in Kafka topic partitions and is fully recoverable on restart.

**Exactly-once game-over**: `ProduceSync("game.broadcast", "game-over", ...)` is called once in `runTurnProcessor` when `result.Winner != ""`, after which the goroutine returns. No subsequent turns are processed.

---

## 10. Analysis Pipelines

### Pipeline 1 — Route Risk Analysis (Light Side)

Four worker goroutines compute risk scores for each possible escape route from the Ring Bearer's current position:

- Counts blocked/threatened paths on route
- Weighs Nazgûl proximity to each path endpoint
- Scores surveillance levels on each path segment
- Returns ranked routes with recommended choice (⭐)

### Pipeline 2 — Interception Analysis (Dark Side)

Four worker goroutines compute optimal Nazgûl deployment targets:

- Estimates Ring Bearer's likely next positions from last-known location
- Scores each Nazgûl's distance to intercept points
- Returns ranked interception opportunities with target region

---

## 11. Unit Tests (13 Tests, Race Detector)

```
=== RUN   TestCombat_TiePlains              PASS
=== RUN   TestCombat_FortressTerrain        PASS
=== RUN   TestCombat_UrukHaiIgnoresFortress PASS
=== RUN   TestCombat_UrukHaiVsFortified     PASS
=== RUN   TestCombat_LeadershipBonus        PASS
=== RUN   TestCombat_IndestructibleUnit     PASS
=== RUN   TestPipeline1_KnownThreat         PASS
=== RUN   TestPipeline1_NazgulProximity     PASS
=== RUN   TestPipeline2_PositiveIntercept   PASS
=== RUN   TestPipeline2_NegativeIntercept   PASS
=== RUN   TestRouter_DarkSideNeverSeesRB    PASS
=== RUN   TestRouter_RingBearerMovedEvent   PASS
=== RUN   TestRouter_DarkViewAlwaysEmpty    PASS
ok  github.com/rotr/ring-of-the-middle-earth/tests  (race: PASS)
```

Tests run without Docker — no Kafka dependency. `go test -race` directly verifies the information asymmetry invariant: `DarkView.RingBearerRegion` is always `""` under concurrent access.

---

## 12. Bugs Found and Fixed During Development

During integration testing, four bugs were discovered and fixed:

| # | Bug | Root Cause | Fix |
|---|---|---|---|
| 1 | Kafka brokers not connecting | `brokers` env var (CSV) treated as single host | `strings.Split(brokers, ",")` in producer and consumer |
| 2 | `game.js` served as 404 text | nginx regex `~ ^/game` matched `game.js` before static file handler | Changed regex to `~ ^/(order$\|orders/\|game/\|analysis/\|health$)` |
| 3 | `selectPlayer is not defined` | `const` declaration inside `switch case` without braces — strict-mode parse error crashed entire script | Wrapped case body in `{}` |
| 4 | SSE reconnecting loop | nginx variable proxy `http://$sse_backend` needs DNS resolver; no resolver defined | Added `resolver 127.0.0.11 valid=30s ipv6=off` |
| 5 | Ring Bearer never moves | `ProcessTurn` takes `TurnState` by value; caller's `state.RingBearer` unchanged. `cache.Update` never persisted `RingBearerState` | Added `RingBearerState` field to `TurnResult`; `cache.Update` now saves full state |
| 6 | Ring Bearer route reset every tick | `applyCacheUpdate` called `worldCache.Update` with empty `TurnResult{RingBearerState:{}}`, wiping Route/RouteIdx | Added `UpdateFromBroadcast` method that syncs only units/regions without touching `RingBearerState` |
| 7 | Turn-2+ orders fail `WRONG_TURN` | `validator.ResetTurnOrders()` cleared `OrdersThisTurn` but never advanced `CurrentTurn` | Replaced with `validator.SetKTable(buildKTable(worldCache))` which rebuilds KTable including correct `CurrentTurn` |

---

## 13. End-to-End Gameplay Verification

The following sequence was verified live in two browser tabs:

| Turn | Ring Bearer Position | Action |
|---|---|---|
| 1 | The Shire | Light Side: `ASSIGN_ROUTE` (9-hop route to Mount Doom) |
| 2 | Bree | Auto-advance |
| 3 | Weathertop | Auto-advance |
| 4 | Rivendell | Auto-advance |
| 5 | Moria | Dark Side: `DEPLOY_NAZGUL` targeting Lothlórien |
| 6 | Lothlórien | Auto-advance |
| 7 | Emyn Muil | Auto-advance |
| 8 | Dead Marshes | Auto-advance |
| 9 | Mordor | Auto-advance |
| 10 | **Mount Doom** | Light Side wins — `RING_DESTROYED` |

At all turns, `curl /game/state?playerId=dark | jq '.ringBearer.currentRegion'` returned `""` — Dark Side had no knowledge of the Ring Bearer's position.

---

## 14. Technology Choice Rationale (Go vs Akka)

**Why Go was chosen:**

- Goroutines map directly to the specification's goroutine architecture diagram — no translation layer between design and implementation
- Stateless application tier (state lives in Kafka KTables) eliminates the need for actor persistence frameworks or cluster sharding
- Fault tolerance is fully delegated to Kafka's consumer group rebalance protocol
- `go test -race` is a first-class tool that directly verifies the information asymmetry invariant at the language level

**Where Akka would have advantages:**

- *Turn processing atomicity*: A `PersistentActor` with event-sourcing would make the 13-step turn a resumable `persist` chain, recoverable from crash by journal replay. The Go implementation uses Kafka topic replay as an equivalent but coarser mechanism.
- *Information hiding*: `RingBearerActor` as a `ClusterSingleton` would enforce position secrecy at the type system level. The Go implementation enforces it through `EventRouter` as a single gatekeeping goroutine — correct but relies on code discipline rather than compiler enforcement.

---

## 15. Running the Project

```bash
# Start full system (Kafka × 3, ZooKeeper, Schema Registry, Go × 3, nginx)
make up

# Run unit tests (no Docker required)
make test

# Open game
open http://localhost
# Tab 1: choose Light Side
# Tab 2: choose Dark Side

# Demo: fault tolerance
make demo-kill-go2   # stop go-2, watch rebalance
make demo-start-go2  # rejoin

# Verify information hiding
curl "http://localhost/game/state?playerId=dark" | jq '.ringBearer'
# Expected: { "currentRegion": "", "lastDetectedRegion": "", "lastDetectedTurn": 0 }
```

---

## 16. Repository Structure

```
ring-of-the-middle-earth/
├── docker-compose.yml
├── nginx.conf
├── Makefile
├── config/
│   ├── units.conf          ← 14 unit configs (no ID literals in logic)
│   └── map.conf            ← 22 regions + 37 paths
├── kafka/
│   ├── schemas/            ← 13 Avro schemas
│   └── init/               ← topic + schema registry init script
├── option-b/               ← Go implementation (~1,300 lines, 21 files)
│   ├── cmd/server/main.go  ← goroutine wiring, select loop, turn processor
│   └── internal/
│       ├── config/         ← HOCON config loader
│       ├── game/           ← types, graph, combat, detection, turn, orders
│       ├── kafka/          ← producer, consumer, order validator (KTable)
│       ├── cache/          ← WorldStateCache (mutex-protected, view-split)
│       ├── router/         ← EventRouter (information asymmetry enforcement)
│       ├── pipeline/       ← Pipeline 1 (route risk) + Pipeline 2 (intercept)
│       └── api/            ← HTTP handlers + SSE server
│   └── tests/              ← 13 unit tests, race detector
└── ui/
    ├── index.html
    ├── game.js             ← map rendering, SSE client, order form
    └── style.css
```
