# Ring of the Middle Earth

**Technology Choice: Option B — Go**

A distributed, browser-based turn-based strategy game. Two players compete in separate browsers: Light Side escorts the Ring Bearer to Mount Doom, Dark Side hunts it down.

---

## Quick Start

```bash
# Start the full system (Kafka x3, ZooKeeper, Schema Registry, Go x3, nginx)
make up

# Run unit tests (no Docker required)
make test

# View logs
make logs
```

Open **http://localhost** in two browser tabs. Choose Light Side in one, Dark Side in the other.

---

## How to Play

### Setup
1. Run `make up` and wait ~30 seconds for all services to start.
2. Open **http://localhost** in **two separate browser tabs**.
3. Click **Light Side** in one tab, **Dark Side** in the other.

### Goal
| Side | Win condition |
|---|---|
| ☀️ Light Side | Ring Bearer reaches **Mount Doom** |
| 🌑 Dark Side | A Nazgul detects and captures the **Ring Bearer** |

### Each Turn
Submit one or more orders using the **Submit Order** panel on the right:

1. **Select a unit** from the dropdown (or click a unit card in "Your Units").
2. **Select an order type** — the form adapts to show only the relevant fields.
3. Fill in the required field(s) and click **Submit Order**.

The turn processor fires every 30 seconds and resolves all submitted orders.

### Order Types

| Order | Who | What it does |
|---|---|---|
| `ASSIGN_ROUTE` | Light | Set the Ring Bearer's escape route (comma-separated path IDs) |
| `REDIRECT_UNIT` | Both | Change a moving unit's path mid-route |
| `ATTACK_REGION` | Both | Move a military unit into a region to start combat |
| `REINFORCE_REGION` | Both | Increase defense strength of a friendly region |
| `MAIA_ABILITY` | Both | **Gandalf** → opens a blocked path; **Saruman** → corrupts a path |
| `BLOCK_PATH` | Dark | Nazgul plants a blockade on a path |
| `SEARCH_PATH` | Dark | Nazgul searches a path for the Ring Bearer |
| `DEPLOY_NAZGUL` | Dark | Move a Nazgul to a target region |

### Map Legend
| Symbol | Meaning |
|---|---|
| 🟢 Green line | Path is **open** |
| 🟠 Orange line | Path is **threatened** |
| 🔴 Red line | Path is **blocked** |
| 🔵 Dashed line | Path is **temporarily open** |
| Blue dot | Light Side unit |
| Red dot | Dark Side unit |
| 💍 Ring icon | Ring Bearer (Light Side only sees this) |
| 👁 Eye icon | Last known Ring Bearer position (Dark Side) |

### Panels
- **Your Units** — lists all units with region and strength. Click to auto-fill the order form.
- **Submit Order** — fill in unit, order type, and target fields. Click a region or path on the map to auto-fill target inputs.
- **Route Risk Analysis** (Light Side) — click "Refresh Analysis" to see risk scores for each escape route. ⭐ = recommended.
- **Interception Analysis** (Dark Side) — shows which Nazgul units have the best intercept opportunity and where to move.
- **Event Log** — live stream of all game events via SSE.

### Tips
- **Light Side**: Use "Refresh Analysis" every few turns. The recommended route changes as Dark Side blocks paths. Keep Gandalf available to re-open critical paths.
- **Dark Side**: Deploy Nazgul near the Ring Bearer's likely route early. Use `SEARCH_PATH` to trigger detection checks. Saruman can corrupt paths to force the Ring Bearer into a trap.

---

## Architecture

```
Browser A (Light)            Browser B (Dark)
  POST /order                  POST /order
  GET  /events (SSE)           GET  /events (SSE)
  GET  /analysis/routes        GET  /analysis/intercept
        |                            |
        +----------[ nginx ]---------+
                       |
           +-----------+-----------+
           |   3 × Go instances    |
           |  go-1  go-2  go-3     |
           |  (stateless HTTP +    |
           |   SSE + game engine)  |
           +-----------+-----------+
                       |
           +-----------+-----------+
           |         Kafka         |
           |  3-broker cluster     |
           |  10 topics            |
           |  Schema Registry      |
           +-----------+-----------+
```

### Go instance goroutines (Section 28)

Each instance runs:

| Goroutine | Role |
|---|---|
| KafkaConsumer × N | One per subscribed topic; feeds `eventCh` |
| EventRouter | Routes events to Light/Dark SSE channels and engine; enforces information asymmetry |
| CacheManager | Owns `WorldStateCache`; delivers value copies, never pointers |
| TurnProcessor | 13-step turn processing; produces events to Kafka |
| Pipeline 1 — Route Risk | 4 workers, cap-20 buffer; computes route risk scores for Light Side |
| Pipeline 2 — Interception | 4 workers, cap-30 buffer; computes intercept plans for Dark Side |
| SSE goroutines | One per connected player |
| HTTP server | REST API |
| Main select loop | 7 cases: raw orders, new connections, disconnects, analysis, cache updates, turn tick, OS signal |

### Kafka topics (10 total)

| Topic | Partitions | Cleanup | Key |
|---|---|---|---|
| game.orders.raw | 3 | delete 1h | playerId |
| game.orders.validated | 6 | delete 1h | unitId |
| game.events.unit | 6 | delete 7d | unitId |
| game.events.region | 6 | delete 7d | regionId |
| game.events.path | 6 | delete 7d | pathId |
| game.session | 1 | compact | — |
| game.broadcast | 1 | delete 1h | — |
| game.ring.position | 1 | delete 1h | — |
| game.ring.detection | 2 | delete 1h | playerId |
| game.dlq | 3 | delete 7d | errorCode |

### Information Asymmetry

`EventRouter` is the **single enforcement point**. `game.ring.position` never reaches the Dark Side SSE channel. `WorldStateSnapshot` is stripped of `ring-bearer.currentRegion` before delivery to the Dark Side. `DarkView.RingBearerRegion` is always `""` — enforced in code and verified by `go test -race`.

### Fault Tolerance (Demo Scenario 3)

```bash
docker compose stop go-2   # observe consumer group rebalance in Kafka logs
# game continues on go-1 and go-3
docker compose start go-2  # go-2 rejoins, replays partition state from Kafka
```

---

## Why Go over Akka?

**Go is well-suited here because:**
- Goroutines map directly onto the spec's goroutine architecture diagram (Section 28) — no translation layer.
- The stateless application tier (state lives in Kafka KTables) eliminates the need for cluster sharding or actor persistence frameworks.
- Fault tolerance is entirely delegated to Kafka's consumer group protocol — simpler to reason about than Akka's supervision trees.
- `go test -race` is a first-class tool that directly verifies the information asymmetry invariant.

**What is harder with Go than with Akka:**
- No built-in message persistence — we rely on Kafka topic replay instead of LevelDB actor journals.
- Supervision/restart logic must be written by hand (retry loops, backoff) rather than declared in a supervision strategy.
- Cluster-local state queries (e.g. asking "where is unit X?") require a round-trip through Kafka rather than an actor ask pattern.

**How Akka would solve the two hardest parts:**
1. *Turn processing atomicity*: `WorldStateActor` as a `PersistentActor` with event-sourcing — the 13 steps become a single `persist` chain, resumable after crash by replaying the journal.
2. *Information hiding*: `RingBearerActor` is a `ClusterSingleton` — its `trueRegion` field is never in a shared message unless explicitly addressed to a Light-Side-only consumer, enforced by the actor's `receive` function.

---

## Running the Demo Scenarios

### Scenario 1 — Information Hiding

```bash
# Light Side browser: observe ring bearer position in /game/state?playerId=light
# Dark Side browser: currentRegion must be "" in /game/state?playerId=dark
curl "http://localhost/game/state?playerId=dark" | jq '.ringBearer'
# Expected: { "currentRegion": "", "lastDetectedRegion": "...", ... }
```

### Scenario 2 — Maia Dispatch

Both Gandalf and Saruman respond to the same `MAIA_ABILITY` order type. The game engine dispatches to different effects purely by reading `config.MaiaAbilityPaths`: empty → OpenPath (Gandalf), non-empty → CorruptPath (Saruman). No unit ID string literal appears in the dispatch logic.

```bash
# Gandalf: open a blocked path
curl -X POST http://localhost/order -H "Content-Type: application/json" \
  -d '{"orderType":"MAIA_ABILITY","playerId":"light","unitId":"gandalf","turn":5,"targetPathId":"shire-to-bree"}'

# Saruman: corrupt a southern corridor path
curl -X POST http://localhost/order -H "Content-Type: application/json" \
  -d '{"orderType":"MAIA_ABILITY","playerId":"dark","unitId":"saruman","turn":5,"targetPathId":"fords-of-isen-to-edoras"}'
```

### Scenario 3 — Fault Tolerance + Exactly-Once

```bash
make demo-kill-go2   # docker compose stop go-2
# Watch: docker compose logs kafka-1 | grep "rebalance"
# Game continues on go-1 and go-3

make demo-start-go2  # docker compose start go-2

# Verify GameOver exactly once:
make inspect-broadcast
```

---

## Unit Tests

```bash
make test
# === RUN   TestCombat_TiePlains              PASS
# === RUN   TestCombat_FortressTerrain        PASS
# === RUN   TestCombat_UrukHaiIgnoresFortress PASS
# === RUN   TestCombat_UrukHaiVsFortified     PASS
# === RUN   TestCombat_LeadershipBonus        PASS
# === RUN   TestCombat_IndestructibleUnit     PASS
# === RUN   TestPipeline1_KnownThreat...      PASS
# === RUN   TestPipeline1_NazgulProximity     PASS
# === RUN   TestPipeline2_PositiveIntercept   PASS
# === RUN   TestPipeline2_NegativeIntercept   PASS
# === RUN   TestRouter_DarkSideNever...       PASS
# === RUN   TestRouter_RingBearerMoved...     PASS
# === RUN   TestRouter_DarkViewAlwaysEmpty    PASS
# ok  github.com/rotr/ring-of-the-middle-earth/tests  (race: PASS)
```

---

## Repository Structure

```
ring-of-the-middle-earth/
├── docker-compose.yml
├── nginx.conf
├── Makefile
├── README.md
├── config/
│   ├── units.conf          ← 14 unit configs (no ID literals in logic)
│   └── map.conf            ← 22 regions + 37 paths
├── kafka/
│   ├── schemas/            ← 13 Avro schemas
│   └── init/               ← topic + schema registry init script
├── option-b/               ← Go implementation
│   ├── Dockerfile
│   ├── go.mod
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── config/         ← HOCON config loader
│   │   ├── game/           ← types, graph, combat, detection, turn, orders
│   │   ├── kafka/          ← producer, consumer, order validator
│   │   ├── cache/          ← WorldStateCache
│   │   ├── router/         ← EventRouter (information asymmetry)
│   │   ├── pipeline/       ← Pipeline 1 (route risk) + Pipeline 2 (intercept)
│   │   └── api/            ← HTTP handlers + SSE server
│   └── tests/              ← 13 unit tests (run without Docker)
└── ui/
    ├── index.html
    ├── game.js
    └── style.css
```
