# Presentation Transcript — Ring of the Middle Earth
## 15-minute live demo + 5-minute Q&A

---

## BEFORE YOU START (Pre-Demo Setup — ~5 minutes, NOT counted in the 15)

### Step 0 — Reduce turn duration to 30 seconds (faster demo)

```bash
# Edit config: change turn-duration-seconds from 60 to 30
# In file: config/units.conf, last line:
sed -i '' 's/turn-duration-seconds = 60/turn-duration-seconds = 30/' \
  /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth/config/units.conf
```

### Step 1 — Clean restart

```bash
cd /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth
docker compose down -v          # wipe volumes (clears stale Kafka/ZK state)
make up                         # start full system
```

Wait ~30 seconds for all services healthy:
```bash
docker compose ps               # all services should show "healthy" or "Up"
curl http://localhost/health     # should return {"status":"ok"}
```

### Step 2 — Open two browser tabs

```
Tab 1: http://localhost  → click LIGHT SIDE
Tab 2: http://localhost  → click DARK SIDE
```

Arrange both windows side-by-side on screen.

### Step 3 — Position units for Scenario 1 (submit these BEFORE 15-min clock)

**Get current turn (should be 1):**
```bash
curl -s "http://localhost/game/state?playerId=light" | python3 -c \
  "import sys,json; print('Turn:', json.load(sys.stdin)['turn'])"
```

**Light Side — route Ring Bearer toward Weathertop:**
```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d '{
  "orderType":"ASSIGN_ROUTE",
  "playerId":"light",
  "unitId":"ring-bearer",
  "turn":1,
  "pathIds":["shire-to-bree","bree-to-weathertop"]
}'
# Expected: {"status":"accepted"}
```

**Dark Side — route Witch-King north toward Ring Bearer:**
```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d '{
  "orderType":"ASSIGN_ROUTE",
  "playerId":"dark",
  "unitId":"witch-king",
  "turn":1,
  "pathIds":[
    "osgiliath-to-minas-morgul",
    "minas-tirith-to-osgiliath",
    "rohan-plains-to-minas-tirith",
    "lothlorien-to-rohan-plains"
  ]
}'
# Expected: {"status":"accepted"}
```

**Wait ~2.5 minutes (5 turns × 30s) for unit positioning to complete:**
```bash
# Poll until turn 5:
until [ "$(curl -s 'http://localhost/game/state?playerId=light' | \
  python3 -c 'import sys,json; print(json.load(sys.stdin)["turn"])')" -ge 5 ]; do
  sleep 5
done && echo "READY — start 15-min clock now"
```

After turn 4 processing:
- Ring Bearer is at **Weathertop** (arrived turn 3, stays there)
- Witch-King is at **Lothlórien** (2 hops from Weathertop — within detection range 3)

---

## ▶ START 15-MINUTE CLOCK

---

## SECTION A — Distributed Architecture (2 minutes)

**What to say and show:**

Open a terminal. Run:
```bash
docker compose ps
```

**Say:** "The system runs on Docker, simulating a distributed cluster. We have:
- 3 Kafka brokers forming a cluster — no single point of failure
- ZooKeeper for Kafka coordination
- Confluent Schema Registry validating 13 Avro schemas
- 3 stateless Go application instances (go-1, go-2, go-3) behind nginx
- nginx acting as load balancer and SSE sticky router"

Show nginx config:
```bash
cat nginx.conf | grep -A5 "upstream\|sse_backend"
```

**Say:** "Regular API calls are round-robined across all three Go instances. SSE connections are pinned by player ID — Light Side always connects to go-1, Dark Side to go-2. This ensures each player gets a consistent stream of game events."

Show live:
```bash
curl http://localhost/health          # hits one of go-1/go-2/go-3
docker compose logs --tail=5 go-1    # show startup logs
```

**Say:** "Each Go instance runs 9 goroutine types concurrently — Kafka consumers, EventRouter, CacheManager, TurnProcessor, two fan-out analysis pipelines, SSE goroutines, and an HTTP server. No shared mutable state crosses goroutine boundaries without a channel or mutex."

---

## SECTION B — Code Walk (2 minutes)

Show these files briefly (have them open in editor):

**1. `option-b/internal/router/router.go` — information asymmetry enforcement point:**
```bash
cat option-b/internal/router/router.go | head -50
```
**Say:** "EventRouter is the ONLY place in the codebase where topic routing happens. `game.ring.position` goes to Light Side only. `game.ring.detection` goes to Dark Side only. `game.broadcast` has ring-bearer stripped before delivery to Dark Side. One function, verified by tests with the race detector."

**2. `option-b/internal/game/turn.go` line 34 — pure function:**
**Say:** "ProcessTurn is a pure function. It takes a world state snapshot and a list of validated orders, runs 13 deterministic steps, and returns a TurnResult. No I/O, no side effects. This is what makes Kafka replay possible — you can reconstruct any game state by replaying orders."

**3. `option-b/internal/kafka/validator.go` — Kafka Streams Topology 1:**
**Say:** "OrderValidator implements a KTable-based order validation pipeline. It checks 8 rules: correct turn, unit ownership, path availability, adjacency, cooldowns. Valid orders produce to `game.orders.validated`; invalid orders go to `game.dlq`. After each turn the KTable is rebuilt from the updated cache so turn-2+ orders validate correctly."

---

## SCENARIO 1 — Information Hiding (5 minutes)

> **State check first — confirm positions:**

```bash
# Light Side sees Ring Bearer at Weathertop
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Turn:', d['turn']); print('RB:', d['ringBearer'])"

# Dark Side sees empty position
curl -s "http://localhost/game/state?playerId=dark" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Turn:', d['turn']); print('RB:', d['ringBearer'])"
```

**Expected output:**
```
Light Side → RB: {'currentRegion': 'weathertop'}
Dark Side  → RB: {'currentRegion': '', 'lastDetectedRegion': '', 'lastDetectedTurn': 0}
```

**Say:** "Light Side can see the Ring Bearer at Weathertop. Dark Side sees empty string — the position is completely hidden. This is enforced in EventRouter, in WorldStateCache.GetPublicState, and by never publishing to game.ring.position on the Dark Side SSE channel."

> **Show detection firing:**

Get current turn:
```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
echo "Current turn: $TURN"
```

**Point at both browser tabs side-by-side.**

Submit a SEARCH_PATH order to raise surveillance on the path Ring Bearer is on, then watch the Event Log:
```bash
# Dark Side: Witch-King searches path Ring Bearer will cross next turn
# (witch-king is now at lothlorien, adjacent to moria-to-lothlorien)
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"SEARCH_PATH\",
  \"playerId\":\"dark\",
  \"unitId\":\"witch-king\",
  \"turn\":$TURN,
  \"pathId\":\"moria-to-lothlorien\"
}"
```

Wait 30 seconds for the turn to fire. **Point at browser tabs.**

**After tick:**
- Dark Side Event Log: shows `RING_BEARER_DETECTED` or `RingBearerSpotted` event
- Light Side Event Log: does NOT show this event

```bash
# Verify via API:
curl -s "http://localhost/game/state?playerId=dark" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); print('DarkView:', d['ringBearer'])"
# Expected: lastDetectedRegion shows a value now
```

**Say:** "Dark Side received the detection event. Light Side did not. The EventRouter enforces this at the channel level — there is no code path that can accidentally send ring-bearer position to the Dark Side SSE channel."

**Verify with curl (show instructor running this live):**
```bash
# Three independent enforcement points:
echo "=== 1. REST API (DarkView) ===" && \
curl -s "http://localhost/game/state?playerId=dark" | python3 -m json.tool | grep -A3 "ringBearer"

echo "=== 2. REST API (LightView) ===" && \
curl -s "http://localhost/game/state?playerId=light" | python3 -m json.tool | grep -A3 "ringBearer"
```

---

## SCENARIO 2 — Maia Dispatch and Path Mechanics (5 minutes)

> **Get current turn:**
```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
echo "Current turn: $TURN"
```

> **Step 1: Block a path (Dark Side — Saruman at Isengard blocks adjacent path)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"BLOCK_PATH\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"pathId\":\"fangorn-to-isengard\"
}"
# Expected: {"status":"accepted"}
```

> **Step 2: Same turn — Gandalf opens the blocked path (Maia Dispatch Demo)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"light\",
  \"unitId\":\"gandalf\",
  \"turn\":$TURN,
  \"targetPathId\":\"fangorn-to-isengard\"
}"
# Expected: {"status":"accepted"}
```

> **Step 3: Same turn — Saruman corrupts a different path (PathCorrupted demo)**

```bash
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"targetPathId\":\"fords-of-isen-to-edoras\"
}"
# Wait — saruman already used BLOCK_PATH this turn. Let's check if duplicate order is rejected:
# Actually BLOCK_PATH uses pathId, MAIA_ABILITY uses targetPathId — different units in different turns
# Saruman submitted BLOCK_PATH. For MAIA_ABILITY, use a different turn or note that saruman 
# has duplicate-unit-order protection. 
# CORRECTION: Submit Saruman MAIA_ABILITY on the NEXT turn instead.
```

**NOTE FOR INSTRUCTOR:** Saruman can only submit ONE order per turn. Submit the PathCorrupt on the next turn:

```bash
# Wait for turn to advance, then:
TURN=$((TURN + 1))
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"dark\",
  \"unitId\":\"saruman\",
  \"turn\":$TURN,
  \"targetPathId\":\"fords-of-isen-to-edoras\"
}"
# Expected: {"status":"accepted"}
```

**Wait for turn to fire. Show browser map.**

> **What to observe on the map after turn fires:**

- `fangorn-to-isengard` → shows as **blue dashed** (TEMPORARILY_OPEN) in both browsers
- `fords-of-isen-to-edoras` → shows as **orange** (Threatened/high surveillance) after PathCorrupt

```bash
# Verify path states:
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "
import sys,json; d=json.load(sys.stdin)
for p in d['paths']:
    if p['id'] in ['fangorn-to-isengard','fords-of-isen-to-edoras']:
        print(p['id'], '→', p['status'], 'surveillance:', p['surveillanceLevel'])
"
```

**Say:** "Both Gandalf and Saruman use the exact same order type: `MAIA_ABILITY`. The game engine dispatches to different effects purely by reading the unit's config — Gandalf has `maiaAbilityPaths = []` (empty), which means OpenPath behavior. Saruman has a non-empty list, which triggers CorruptPath. No unit ID string literals appear anywhere in the dispatch logic."

> **Show code briefly:**
```bash
grep -A8 "MaiaAbilityPaths" option-b/internal/game/turn.go | head -15
```

**Say:** "If `len(cfg.MaiaAbilityPaths) == 0` → Gandalf behavior. Otherwise → Saruman behavior. The config drives all dispatch."

> **Wait 2 more turns — show Gandalf path reverting:**

```bash
# After 2 turns from when Gandalf opened the path:
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "
import sys,json; d=json.load(sys.stdin)
for p in d['paths']:
    if p['id'] == 'fangorn-to-isengard':
        print('fangorn-to-isengard status:', p['status'])
"
# Expected: status back to BLOCKED (saruman still at isengard endpoint)
```

**Say:** "Gandalf's path was temporarily open for exactly 2 turns, then reverted because the blocking unit (Saruman) was still at the path endpoint. Saruman's corruption of fords-of-isen-to-edoras however is permanent — SurveillanceLevel raised to 3, it never resets."

> **Show DLQ to prove the duplicate-order rejection:**
```bash
# Try submitting Gandalf again (cooldown = 3 turns)
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"MAIA_ABILITY\",
  \"playerId\":\"light\",
  \"unitId\":\"gandalf\",
  \"turn\":$TURN,
  \"targetPathId\":\"fangorn-to-isengard\"
}"
# Expected: order accepted but will be REJECTED by validator → goes to DLQ
# Then show DLQ:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.dlq --from-beginning --timeout-ms 3000 2>/dev/null | \
  python3 -c "import sys; [print(l) for l in sys.stdin if 'COOLDOWN' in l or 'DLQ' in l]"
```

---

## SCENARIO 3 — Fault Tolerance and Exactly-Once (5 minutes)

### Part A: Route Ring Bearer to Mount Doom

```bash
TURN=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['turn'])")
RB=$(curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['ringBearer']['currentRegion'])")
echo "Turn: $TURN, Ring Bearer at: $RB"
```

Submit full route to Mount Doom from current Ring Bearer position (adjust starting path based on where RB is):

```bash
# Ring Bearer should be at weathertop. Route:
curl -X POST http://localhost/order -H "Content-Type: application/json" -d "{
  \"orderType\":\"ASSIGN_ROUTE\",
  \"playerId\":\"light\",
  \"unitId\":\"ring-bearer\",
  \"turn\":$TURN,
  \"pathIds\":[
    \"weathertop-to-rivendell\",
    \"rivendell-to-moria\",
    \"moria-to-lothlorien\",
    \"lothlorien-to-emyn-muil\",
    \"emyn-muil-to-dead-marshes\",
    \"dead-marshes-to-mordor\",
    \"mordor-to-mount-doom\"
  ]
}"
# Expected: {"status":"accepted"}
```

**Say:** "Ring Bearer has 7 steps to Mount Doom. Each turn it auto-advances one step. With 30-second turns, this takes 3.5 minutes. While we wait, I'll explain the consumer group architecture."

**While waiting, explain (show docker logs):**
```bash
# Show 3 separate consumer groups
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-consumer-groups --bootstrap-server localhost:9092 --list
```

**Say:** "Each Go instance subscribes with its own consumer group ID: game-engine-1, game-engine-2, game-engine-3. This means ALL THREE instances receive every validated order and process it independently. State lives in Kafka — the Go instances are stateless. This is why killing one instance doesn't lose any data."

### Part B: Kill go-2 During Turn Processing

**Wait until Ring Bearer is 1-2 turns from Mount Doom:**
```bash
# Poll for Ring Bearer approaching mordor
until curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; rb=json.load(sys.stdin)['ringBearer']['currentRegion']; \
  print(rb); exit(0 if rb in ['mordor','dead-marshes'] else 1)" 2>/dev/null; do
  sleep 5
done && echo "Ring Bearer near Mount Doom — ready to kill go-2"
```

**Kill go-2:**
```bash
docker compose stop go-2
echo "go-2 stopped"
```

**Immediately show Kafka rebalance in logs:**
```bash
docker compose logs kafka-1 --follow 2>&1 | grep -i "rebalance\|join\|leader" &
# Let it run for ~10 seconds then Ctrl+C
```

**Say:** "Kafka's consumer group protocol detects that game-engine-2 has gone offline. It triggers a rebalance — redistributing go-2's partitions to go-1 and go-3. This takes under 10 seconds. The game continues uninterrupted on the remaining two instances."

**Verify game continues:**
```bash
sleep 35  # wait for next turn tick
curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('Turn:', d['turn'], '| RB:', d['ringBearer']['currentRegion'])"
```

**Expected:** Turn incremented, Ring Bearer moved — proof that go-1 and go-3 continued processing without go-2.

### Part C: Ring Bearer Reaches Mount Doom — Game Over

```bash
# Poll until mount-doom
until curl -s "http://localhost/game/state?playerId=light" | \
  python3 -c "import sys,json; \
  rb=json.load(sys.stdin)['ringBearer']['currentRegion']; \
  exit(0 if rb == 'mount-doom' else 1)" 2>/dev/null; do sleep 5; done
echo "Ring Bearer at Mount Doom!"

# Check game.broadcast for GameOver event:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic game.broadcast \
  --from-beginning \
  --timeout-ms 3000 2>/dev/null | \
  python3 -c "
import sys, json
count = 0
for line in sys.stdin:
    try:
        d = json.loads(line)
        if 'winner' in d or 'game-over' in str(d):
            count += 1
            print(f'GameOver event #{count}:', d)
    except:
        pass
print(f'Total GameOver events in game.broadcast: {count}')
"
```

**Expected output:**
```
GameOver event #1: {'winner': 'FREE_PEOPLES', 'cause': 'RING_DESTROYED', 'turn': N, ...}
Total GameOver events in game.broadcast: 1
```

**Say:** "Exactly one GameOver event in game.broadcast. This is because runTurnProcessor calls ProduceSync exactly once when result.Winner is non-empty, then immediately returns — the goroutine exits. No further turns can be processed. The Kafka topic is the durable record."

### Part D: Restart go-2 and Verify Recovery

```bash
docker compose start go-2
sleep 5
echo "go-2 restarted"

# Verify go-2 rejoined consumer group:
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-consumer-groups --bootstrap-server localhost:9092 \
  --group game-engine-2 --describe 2>/dev/null | head -10
```

```bash
# Verify go-2 has correct game state:
curl -s "http://go-2:8080/game/state?playerId=light" 2>/dev/null | \
  python3 -c "import sys,json; d=json.load(sys.stdin); \
  print('go-2 turn:', d['turn'], '| RB:', d['ringBearer']['currentRegion'])"
```

*(If direct curl to go-2 doesn't work from host, check via Docker network)*

**Say:** "go-2 rejoined the consumer group. Kafka replayed the partition offsets from the point go-2 left off. go-2 reconstructed its WorldStateCache from the event stream — no manual intervention, no data loss. This is the 'state in Kafka' design principle."

---

## SECTION C — Unit Tests (1 minute)

```bash
cd /Users/emreyildiz/distrubuted-2/ring-of-the-middle-earth
make test
```

**Expected output:**
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

**Say:** "13 unit tests, all passing, including the race detector (`-race` flag). The router tests directly assert that DarkView.RingBearerRegion is always empty under concurrent reads and writes — the same invariant we just demonstrated live. Tests run without Docker — no Kafka dependency."

---

## SECTION D — Report (30 seconds)

```bash
# Show report structure:
wc -l REPORT.md    # 358 lines
head -60 REPORT.md  # show table of contents
```

**Say:** "The report covers 16 sections: architecture, the 9-goroutine design, all 10 Kafka topics, the three demo scenarios as code walkthroughs, the 13-step turn processing pipeline, and notably Section 12 — a table of 7 bugs found and fixed during integration testing, each with root cause and fix. The end-to-end gameplay verification section documents the Ring Bearer's path turn-by-turn."

---

## ▶ STOP 15-MINUTE CLOCK

---

## Q&A PREPARATION — Likely Questions

**Q: Why did you choose Go over Akka?**
> "Goroutines map 1:1 to the specification's goroutine diagram. The stateless design with Kafka as the state store eliminates the need for actor persistence frameworks. go test -race directly verifies the information asymmetry invariant. Akka would have given us better turn-processing atomicity via PersistentActor and better information hiding via ClusterSingleton — I discuss both tradeoffs in Section 14 of the report."

**Q: How is exactly-once guaranteed?**
> "runTurnProcessor calls ProduceSync once when result.Winner is non-empty, then the goroutine returns. No subsequent ticks can fire. The Kafka topic is append-only — you can verify with kafka-console-consumer that exactly one GameOver message exists with --from-beginning."

**Q: What happens if go-1 (the primary turn-processor) crashes?**
> "All three instances run independent TurnProcessor goroutines. Kafka's consumer group rebalance within seconds redistributes the partitions. The next instance to receive a validated order on game.orders.validated partition will process the next turn. There's a brief overlap window where two instances might attempt to process the same turn, but since ProcessTurn is pure and idempotent (same input produces same output), the result is consistent."

**Q: How does the Kafka DLQ work?**
> "Any order that fails validation is written to game.dlq with a structured error: originalTopic, errorCode, errorMessage, rawPayload. Error codes are typed constants in the game package: WRONG_TURN, NOT_YOUR_UNIT, INVALID_PATH, PATH_BLOCKED, UNIT_NOT_ADJACENT, DUPLICATE_UNIT_ORDER, ABILITY_ON_COOLDOWN."

**Q: How does the ring-bearer's position stay hidden in the database/log?**
> "The ring bearer's true position is never written to any Kafka topic that the Dark Side consumes. game.ring.position is not in the Dark Side consumer's subscription list. game.broadcast is stripped by stripRingBearer() before delivery to the Dark Side SSE channel. The REST endpoint /game/state?playerId=dark always returns currentRegion='' regardless of actual position."

**Q: Could a malicious client call /game/state?playerId=light to cheat?**
> "In a real deployment you would authenticate players and bind the playerId to a session token. The current implementation trusts the playerId parameter — this is an in-scope simplification since the assessment focuses on distributed systems architecture, not auth."

---

## QUICK REFERENCE — Critical Commands

```bash
# Game state
curl -s "http://localhost/game/state?playerId=light" | python3 -m json.tool
curl -s "http://localhost/game/state?playerId=dark" | python3 -m json.tool

# Submit order (replace values as needed)
curl -X POST http://localhost/order \
  -H "Content-Type: application/json" \
  -d '{"orderType":"ASSIGN_ROUTE","playerId":"light","unitId":"ring-bearer","turn":N,"pathIds":["path-1","path-2"]}'

# Check Kafka topics
docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.broadcast --from-beginning --timeout-ms 3000 2>/dev/null

docker exec ring-of-the-middle-earth-kafka-1-1 \
  kafka-console-consumer --bootstrap-server localhost:9092 \
  --topic game.dlq --from-beginning --timeout-ms 3000 2>/dev/null

# Container control
docker compose stop go-2
docker compose start go-2
docker compose logs kafka-1 --follow

# Unit tests
make test

# Full restart (wipes game state)
docker compose down -v && make up
```

---

## VALID PATH IDs (for submitting orders)

```
shire-to-bree               bree → the-shire / bree
bree-to-weathertop          bree → weathertop
bree-to-rivendell           bree → rivendell
bree-to-tharbad             bree → tharbad
weathertop-to-rivendell     weathertop → rivendell
rivendell-to-moria          rivendell → moria
rivendell-to-lothlorien     rivendell → lothlorien
moria-to-lothlorien         moria → lothlorien
lothlorien-to-emyn-muil     lothlorien → emyn-muil
emyn-muil-to-dead-marshes   emyn-muil → dead-marshes
dead-marshes-to-mordor      dead-marshes → mordor
mordor-to-mount-doom        mordor → mount-doom
fangorn-to-isengard         fangorn ↔ isengard
fords-of-isen-to-edoras     fords-of-isen ↔ edoras
osgiliath-to-minas-morgul   osgiliath ↔ minas-morgul
```

---

## UNIT POSITIONS (Fresh Game Start)

| Unit | Side | Region | Class | Key stat |
|---|---|---|---|---|
| ring-bearer | Light | the-shire | RingBearer | — |
| aragorn | Light | bree | FellowshipGuard | strength 5, leadership |
| legolas | Light | rivendell | FellowshipGuard | strength 3 |
| gimli | Light | rivendell | FellowshipGuard | strength 3 |
| gandalf | Light | rivendell | Maia | opens blocked paths, cooldown 3 |
| rohan-cavalry | Light | edoras | FellowshipGuard | strength 4 |
| gondor-army | Light | minas-tirith | GondorArmy | strength 5, can fortify |
| witch-king | Dark | minas-morgul | Nazgul | detectionRange 2, indestructible |
| nazgul-2 | Dark | minas-morgul | Nazgul | detectionRange 1, respawns |
| nazgul-3 | Dark | minas-morgul | Nazgul | detectionRange 1, respawns |
| saruman | Dark | isengard | Maia | corrupts paths, cooldown 2 |
| uruk-hai-legion | Dark | isengard | UrukHaiLegion | ignoresFortress |
| sauron | Dark | mordor | Maia | indestructible, boosts detection range |
