package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rotr/ring-of-the-middle-earth/internal/api"
	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	appcfg "github.com/rotr/ring-of-the-middle-earth/internal/config"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	kafkalayer "github.com/rotr/ring-of-the-middle-earth/internal/kafka"
	"github.com/rotr/ring-of-the-middle-earth/internal/router"
)

func main() {
	configDir := os.Getenv("CONFIG_DIR")
	if configDir == "" {
		configDir = "../../config"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg, err := appcfg.Load(configDir)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded %d units, %d regions, %d paths",
		len(cfg.Units), len(cfg.Regions), len(cfg.Paths))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worldCache := initCache(cfg)

	// Channels (Section 28)
	eventCh := make(chan router.Event, 100)
	lightSideSSECh := make(chan router.Event, 100)
	darkSideSSECh := make(chan router.Event, 100)
	cacheUpdateCh := make(chan router.Event, 100)
	engineCh := make(chan router.Event, 100)
	rawOrderCh := make(chan []byte, 200)
	newConnectionCh := make(chan struct{}, 10)
	disconnectCh := make(chan struct{}, 10)
	analysisRequestCh := make(chan struct{}, 10)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)

	eventRouter := router.NewEventRouter(lightSideSSECh, darkSideSSECh, cacheUpdateCh, engineCh)

	var producer *kafkalayer.Producer
	producer, err = kafkalayer.NewProducer()
	if err != nil {
		log.Printf("Warning: Kafka producer unavailable (running without Kafka): %v", err)
		producer = nil
	}

	ktable := buildKTable(worldCache)
	validator := kafkalayer.NewOrderValidator(producer, ktable)

	sseServer := api.NewSSEServer(lightSideSSECh, darkSideSSECh)
	httpHandler := api.NewHandler(worldCache, rawOrderCh)
	mux := http.NewServeMux()
	httpHandler.RegisterRoutes(mux, sseServer)

	// Goroutines
	go startKafkaConsumerGoroutines(ctx, eventCh)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventCh:
				eventRouter.Route(event)
			}
		}
	}()

	go runCacheManager(ctx, cacheUpdateCh, worldCache)

	go runTurnProcessor(ctx, engineCh, worldCache, cfg, producer, validator)

	go runMainSelectLoop(ctx, rawOrderCh, newConnectionCh, disconnectCh,
		analysisRequestCh, cacheUpdateCh, signalCh, cancel, cfg, validator)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: withCORS(mux),
	}

	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err2 := srv.ListenAndServe(); err2 != nil && err2 != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err2)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx) //nolint:errcheck
	if producer != nil {
		producer.Flush(5000)
		producer.Close()
	}
	log.Println("Shutdown complete")
}

// runMainSelectLoop implements the 7-case select loop (Section 31).
func runMainSelectLoop(
	ctx context.Context,
	rawOrderCh <-chan []byte,
	newConnectionCh <-chan struct{},
	disconnectCh <-chan struct{},
	analysisRequestCh <-chan struct{},
	cacheUpdateCh <-chan router.Event,
	signalCh <-chan os.Signal,
	cancel context.CancelFunc,
	cfg *appcfg.AppConfig,
	validator *kafkalayer.OrderValidator,
) {
	ticker := time.NewTicker(time.Duration(cfg.GameConfig.TurnDurationSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case raw := <-rawOrderCh:
			validator.Validate(raw)
		case <-newConnectionCh:
			log.Println("New SSE connection")
		case <-disconnectCh:
			log.Println("SSE disconnect")
		case <-analysisRequestCh:
			log.Println("Analysis request received")
		case <-cacheUpdateCh:
			// CacheManager handles actual update; this case prevents starvation
		case tick := <-ticker.C:
			log.Printf("Turn tick: %s", tick.Format(time.RFC3339))
		case sig := <-signalCh:
			log.Printf("Received signal %v", sig)
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func runCacheManager(ctx context.Context, cacheUpdateCh <-chan router.Event, worldCache *cache.WorldStateCache) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-cacheUpdateCh:
			applyCacheUpdate(event, worldCache)
		}
	}
}

func applyCacheUpdate(event router.Event, worldCache *cache.WorldStateCache) {
	if event.Topic != "game.broadcast" {
		return
	}
	data, err := json.Marshal(event.Payload)
	if err != nil {
		return
	}
	var snapshot game.WorldStateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return
	}
	result := game.TurnResult{Turn: snapshot.Turn, Snapshot: snapshot}
	worldCache.Update(result, worldCache.RingBearerState.TrueRegion)
}

func runTurnProcessor(
	ctx context.Context,
	engineCh <-chan router.Event,
	worldCache *cache.WorldStateCache,
	cfg *appcfg.AppConfig,
	producer *kafkalayer.Producer,
	validator *kafkalayer.OrderValidator,
) {
	ticker := time.NewTicker(time.Duration(cfg.GameConfig.TurnDurationSeconds) * time.Second)
	defer ticker.Stop()

	var pendingOrders []game.ValidatedOrder

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-engineCh:
			if event.Topic == "game.orders.validated" {
				data, err := json.Marshal(event.Payload)
				if err != nil {
					continue
				}
				var order game.ValidatedOrder
				if err := json.Unmarshal(data, &order); err != nil {
					continue
				}
				pendingOrders = append(pendingOrders, order)
			}

		case <-ticker.C:
			snap := worldCache.Snapshot()
			if snap.GameOver {
				return
			}

			state := buildTurnState(snap, cfg)
			result := game.ProcessTurn(state, pendingOrders)
			pendingOrders = nil

			worldCache.Update(result, state.RingBearer.TrueRegion)
			if state.SarumanDisabled {
				worldCache.SetSarumanDisabled()
			}

			validator.ResetTurnOrders()

			if producer != nil {
				for _, ev := range result.Events {
					producer.Produce(ev.Topic, ev.Key, ev.Payload) //nolint:errcheck
				}
				producer.Produce("game.broadcast", "world", result.Snapshot) //nolint:errcheck
			}

			if result.Winner != "" || result.Draw {
				winner := "DRAW"
				if result.Winner != "" {
					winner = string(result.Winner)
				}
				gameOver := map[string]interface{}{
					"winner":    winner,
					"cause":     result.WinCause,
					"turn":      result.Turn,
					"timestamp": time.Now().UnixMilli(),
				}
				if producer != nil {
					producer.ProduceSync("game.broadcast", "game-over", gameOver) //nolint:errcheck
				}
				log.Printf("Game over! Winner: %s Cause: %s", winner, result.WinCause)
				return
			}
		}
	}
}

func buildTurnState(snap cache.WorldStateCache, cfg *appcfg.AppConfig) game.TurnState {
	pathSlice := make([]game.PathConfig, 0, len(snap.PathConfigs))
	for _, p := range snap.PathConfigs {
		pathSlice = append(pathSlice, p)
	}
	return game.TurnState{
		Turn:            snap.Turn,
		Units:           snap.Units,
		UnitConfigs:     snap.UnitConfigs,
		Paths:           snap.Paths,
		PathConfigs:     snap.PathConfigs,
		Regions:         snap.Regions,
		RegionConfigs:   snap.RegionConfigs,
		RingBearer:      snap.RingBearerState,
		Graph:           game.NewGraph(pathSlice),
		GameCfg:         cfg.GameConfig,
		SarumanDisabled: snap.SarumanDisabled,
	}
}

func buildKTable(worldCache *cache.WorldStateCache) kafkalayer.OrdersKTable {
	snap := worldCache.Snapshot()
	return kafkalayer.OrdersKTable{
		CurrentTurn: snap.Turn,
		Units:       snap.Units,
		Paths:       snap.Paths,
		Regions:     snap.Regions,
		UnitConfigs: snap.UnitConfigs,
		PathConfigs: snap.PathConfigs,
		PlayerSides: map[string]game.Side{
			game.PlayerLightSide: game.SideFreePeoples,
			game.PlayerDarkSide:  game.SideShadow,
		},
		OrdersThisTurn: make(map[string]bool),
	}
}

func initCache(cfg *appcfg.AppConfig) *cache.WorldStateCache {
	units := make(map[string]game.UnitSnapshot)
	rbRegion := ""
	for id, ucfg := range cfg.Units {
		region := ucfg.StartRegion
		if ucfg.Class == game.ClassRingBearer {
			rbRegion = ucfg.StartRegion
			region = "" // hidden in public state
		}
		units[id] = game.UnitSnapshot{
			ID:       id,
			Region:   region,
			Strength: ucfg.Strength,
			Status:   game.UnitActive,
			Route:    []string{},
		}
	}

	regions := make(map[string]game.RegionState)
	for id, rcfg := range cfg.Regions {
		regions[id] = game.RegionState{
			ID:           id,
			ControlledBy: rcfg.StartControl,
			ThreatLevel:  rcfg.StartThreat,
		}
	}

	paths := make(map[string]game.PathState)
	for id := range cfg.Paths {
		paths[id] = game.PathState{
			ID:     id,
			Status: game.StatusOpen,
		}
	}

	return &cache.WorldStateCache{
		Turn:          1,
		Units:         units,
		Regions:       regions,
		Paths:         paths,
		UnitConfigs:   cfg.Units,
		PathConfigs:   cfg.Paths,
		RegionConfigs: cfg.Regions,
		LightView: cache.LightSideView{
			RingBearerRegion: rbRegion,
		},
		RingBearerState: game.RingBearerState{
			TrueRegion: rbRegion,
		},
	}
}

func startKafkaConsumerGoroutines(ctx context.Context, eventCh chan<- router.Event) {
	topics := []string{
		"game.orders.validated",
		"game.events.unit",
		"game.events.region",
		"game.events.path",
		"game.broadcast",
		"game.ring.position",
		"game.ring.detection",
	}
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = "1"
	}
	consumer, err := kafkalayer.NewConsumer("game-engine-" + instanceID)
	if err != nil {
		log.Printf("Warning: Kafka consumer unavailable: %v", err)
		return
	}
	if err := consumer.Subscribe(topics); err != nil {
		log.Printf("Warning: Failed to subscribe: %v", err)
		return
	}
	consumer.ConsumeLoop(ctx, eventCh)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}
