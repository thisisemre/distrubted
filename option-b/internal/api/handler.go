package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/rotr/ring-of-the-middle-earth/internal/cache"
	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	"github.com/rotr/ring-of-the-middle-earth/internal/pipeline"
)

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	cache         *cache.WorldStateCache
	rawOrderCh    chan<- []byte
	playerSides   map[string]game.Side
}

// NewHandler creates the HTTP handler.
func NewHandler(c *cache.WorldStateCache, rawOrderCh chan<- []byte) *Handler {
	return &Handler{
		cache:      c,
		rawOrderCh: rawOrderCh,
		playerSides: map[string]game.Side{
			game.PlayerLightSide: game.SideFreePeoples,
			game.PlayerDarkSide:  game.SideShadow,
		},
	}
}

// RegisterRoutes wires all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, sseServer *SSEServer) {
	mux.HandleFunc("/game/start", h.handleGameStart)
	mux.HandleFunc("/order", h.handleOrder)
	mux.HandleFunc("/game/state", h.handleGameState)
	mux.HandleFunc("/orders/available", h.handleOrdersAvailable)
	mux.HandleFunc("/analysis/routes", h.handleAnalysisRoutes)
	mux.HandleFunc("/analysis/intercept", h.handleAnalysisIntercept)
	mux.HandleFunc("/events", sseServer.HandleSSE)
	mux.HandleFunc("/health", h.handleHealth)

	// Serve UI files
	mux.Handle("/", http.FileServer(http.Dir("./ui")))
}

func (h *Handler) handleGameStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *Handler) handleOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var order game.Order
	if err := json.NewDecoder(r.Body).Decode(&order); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	data, _ := json.Marshal(order)
	select {
	case h.rawOrderCh <- data:
	default:
		http.Error(w, "order queue full", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) handleGameState(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")
	if playerID == "" {
		playerID = game.PlayerDarkSide // default to restricted view
	}
	state := h.cache.GetPublicState(playerID)
	writeJSON(w, http.StatusOK, state)
}

func (h *Handler) handleOrdersAvailable(w http.ResponseWriter, r *http.Request) {
	unitID := r.URL.Query().Get("unitId")
	playerID := r.URL.Query().Get("playerId")

	snap := h.cache.Snapshot()
	g := game.NewGraph(pathConfigSlice(snap.PathConfigs))

	ctx := game.ValidationContext{
		CurrentTurn: snap.Turn,
		Units:       snap.Units,
		UnitConfigs: snap.UnitConfigs,
		Paths:       snap.Paths,
		PathConfigs: snap.PathConfigs,
		Regions:     snap.Regions,
		Graph:       g,
		PlayerSides: map[string]game.Side{
			game.PlayerLightSide: game.SideFreePeoples,
			game.PlayerDarkSide:  game.SideShadow,
		},
		OrdersThisTurn:  make(map[string]bool),
		SarumanDisabled: snap.SarumanDisabled,
	}

	orders := game.AvailableOrders(unitID, playerID, ctx)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unitId":  unitID,
		"orders":  orders,
	})
}

func (h *Handler) handleAnalysisRoutes(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")
	if playerID != game.PlayerLightSide {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	snap := h.cache.Snapshot()
	result := pipeline.ComputeRouteRisk(context.Background(), snap)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleAnalysisIntercept(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")
	if playerID != game.PlayerDarkSide {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	snap := h.cache.Snapshot()
	result := pipeline.ComputeInterceptPlan(context.Background(), snap)
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	snap := h.cache.Snapshot()
	if snap.GameOver {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "game over"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func pathConfigSlice(m map[string]game.PathConfig) []game.PathConfig {
	result := make([]game.PathConfig, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
