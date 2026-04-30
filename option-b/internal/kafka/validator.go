// Package kafka implements Kafka Streams Topology 1 (order validation)
// and Topology 2 (route risk enrichment) as Go consumers.
package kafka

import (
	"context"
	"encoding/json"
	"log"

	"github.com/rotr/ring-of-the-middle-earth/internal/game"
)

// OrdersKTable maintains in-memory KTable state for order validation.
type OrdersKTable struct {
	CurrentTurn    int
	Units          map[string]game.UnitSnapshot
	Paths          map[string]game.PathState
	Regions        map[string]game.RegionState
	UnitConfigs    map[string]game.UnitConfig
	PathConfigs    map[string]game.PathConfig
	PlayerSides    map[string]game.Side
	OrdersThisTurn map[string]bool
	SarumanDisabled bool
}

// OrderValidator implements Kafka Streams Topology 1.
// Consumes from game.orders.raw, produces to game.orders.validated or game.dlq.
type OrderValidator struct {
	consumer   *Consumer
	producer   *Producer
	ktable     OrdersKTable
}

// NewOrderValidator creates a validator with the given KTable state.
func NewOrderValidator(producer *Producer, kt OrdersKTable) *OrderValidator {
	return &OrderValidator{
		producer: producer,
		ktable:   kt,
	}
}

// SetKTable updates the validator's KTable snapshot (called on each world state update).
func (v *OrderValidator) SetKTable(kt OrdersKTable) {
	v.ktable = kt
}

// ResetTurnOrders clears orders seen this turn (called at turn start).
func (v *OrderValidator) ResetTurnOrders() {
	v.ktable.OrdersThisTurn = make(map[string]bool)
}

// Validate validates a single order and routes it to the correct topic.
func (v *OrderValidator) Validate(orderJSON []byte) {
	var order game.Order
	if err := json.Unmarshal(orderJSON, &order); err != nil {
		v.sendToDLQ("game.orders.raw", "PARSE_ERROR", "cannot parse order", orderJSON)
		return
	}

	ctx := game.ValidationContext{
		CurrentTurn:    v.ktable.CurrentTurn,
		Units:          v.ktable.Units,
		UnitConfigs:    v.ktable.UnitConfigs,
		Paths:          v.ktable.Paths,
		PathConfigs:    v.ktable.PathConfigs,
		Regions:        v.ktable.Regions,
		PlayerSides:    v.ktable.PlayerSides,
		OrdersThisTurn: v.ktable.OrdersThisTurn,
		SarumanDisabled: v.ktable.SarumanDisabled,
	}

	// Graph is built on demand from PathConfigs
	pathSlice := make([]game.PathConfig, 0, len(v.ktable.PathConfigs))
	for _, p := range v.ktable.PathConfigs {
		pathSlice = append(pathSlice, p)
	}
	ctx.Graph = game.NewGraph(pathSlice)

	err := game.ValidateOrder(order, ctx)
	if err != nil {
		if ve, ok := err.(*game.ValidationError); ok {
			v.sendToDLQ("game.orders.raw", string(ve.Code), ve.Message, orderJSON)
		}
		return
	}

	// Mark unit as having order this turn
	v.ktable.OrdersThisTurn[order.UnitID] = true

	validated := game.ValidatedOrder{Order: order}
	if err := v.producer.Produce("game.orders.validated", order.UnitID, validated); err != nil {
		log.Printf("Failed to produce validated order: %v", err)
	}
}

func (v *OrderValidator) sendToDLQ(originalTopic, errorCode, errorMessage string, raw []byte) {
	entry := map[string]interface{}{
		"originalTopic": originalTopic,
		"errorCode":     errorCode,
		"errorMessage":  errorMessage,
		"rawPayload":    string(raw),
	}
	if err := v.producer.Produce("game.dlq", errorCode, entry); err != nil {
		log.Printf("Failed to send to DLQ: %v", err)
	}
}

// RunValidationTopology runs the order validation consumer loop.
func RunValidationTopology(ctx context.Context, validator *OrderValidator, rawOrderCh <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-rawOrderCh:
			if !ok {
				return
			}
			validator.Validate(raw)
		}
	}
}
