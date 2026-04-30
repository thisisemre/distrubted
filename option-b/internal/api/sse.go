package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/rotr/ring-of-the-middle-earth/internal/game"
	"github.com/rotr/ring-of-the-middle-earth/internal/router"
)

// SSEClient represents a connected SSE client.
type SSEClient struct {
	id       string
	playerID string
	ch       chan router.Event
}

// SSEServer manages SSE connections for both sides.
type SSEServer struct {
	mu      sync.Mutex
	clients map[string]*SSEClient

	lightSideSSECh <-chan router.Event
	darkSideSSECh  <-chan router.Event
}

// NewSSEServer creates an SSE server wired to the given channels.
func NewSSEServer(lightCh, darkCh <-chan router.Event) *SSEServer {
	s := &SSEServer{
		clients:        make(map[string]*SSEClient),
		lightSideSSECh: lightCh,
		darkSideSSECh:  darkCh,
	}
	go s.fanoutLight()
	go s.fanoutDark()
	return s
}

// HandleSSE is the HTTP handler for /events.
func (s *SSEServer) HandleSSE(w http.ResponseWriter, r *http.Request) {
	playerID := r.URL.Query().Get("playerId")
	if playerID == "" {
		playerID = game.PlayerDarkSide
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	clientID := fmt.Sprintf("%s-%p", playerID, r)
	client := &SSEClient{
		id:       clientID,
		playerID: playerID,
		ch:       make(chan router.Event, 64),
	}

	s.mu.Lock()
	s.clients[clientID] = client
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientID)
		s.mu.Unlock()
	}()

	// Send initial connection confirmation
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"playerId\":\"%s\"}\n\n", playerID)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-client.ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("SSE marshal error: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// Broadcast sends an event to all clients of the given player side.
func (s *SSEServer) Broadcast(playerID string, event router.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clients {
		if c.playerID == playerID {
			select {
			case c.ch <- event:
			default:
				log.Printf("SSE client %s channel full, dropping event", c.id)
			}
		}
	}
}

func (s *SSEServer) fanoutLight() {
	for event := range s.lightSideSSECh {
		s.Broadcast(game.PlayerLightSide, event)
	}
}

func (s *SSEServer) fanoutDark() {
	for event := range s.darkSideSSECh {
		s.Broadcast(game.PlayerDarkSide, event)
	}
}
