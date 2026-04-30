#!/bin/bash
# Creates all 10 required Kafka topics with correct partition/replication/cleanup config.
set -e

KAFKA_BROKER="${KAFKA_BROKER:-kafka-1:9092}"
REPLICATION=3

wait_for_kafka() {
  echo "Waiting for Kafka broker at $KAFKA_BROKER..."
  until kafka-topics --bootstrap-server "$KAFKA_BROKER" --list > /dev/null 2>&1; do
    sleep 2
  done
  echo "Kafka is ready."
}

create_topic() {
  local name=$1
  local partitions=$2
  local cleanup=$3
  local retention=$4

  echo "Creating topic: $name"
  kafka-topics \
    --bootstrap-server "$KAFKA_BROKER" \
    --create \
    --if-not-exists \
    --topic "$name" \
    --partitions "$partitions" \
    --replication-factor "$REPLICATION" \
    --config "cleanup.policy=$cleanup" \
    ${retention:+--config "retention.ms=$retention"}
}

wait_for_kafka

# 10 topics per Section 9
create_topic "game.orders.raw"      3  "delete" "3600000"
create_topic "game.orders.validated" 6 "delete" "3600000"
create_topic "game.events.unit"     6  "delete" "604800000"
create_topic "game.events.region"   6  "delete" "604800000"
create_topic "game.events.path"     6  "delete" "604800000"
create_topic "game.session"         1  "compact" ""
create_topic "game.broadcast"       1  "delete" "3600000"
create_topic "game.ring.position"   1  "delete" "3600000"
create_topic "game.ring.detection"  2  "delete" "3600000"
create_topic "game.dlq"             3  "delete" "604800000"

echo "All topics created."

# Register Avro schemas in Schema Registry
SCHEMA_REGISTRY="${SCHEMA_REGISTRY_URL:-http://schema-registry:8081}"

register_schema() {
  local subject=$1
  local schema_file=$2
  echo "Registering schema: $subject"
  SCHEMA=$(cat "$schema_file" | tr -d '\n' | sed 's/"/\\"/g')
  curl -s -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
    --data "{\"schema\": \"$SCHEMA\"}" \
    "$SCHEMA_REGISTRY/subjects/$subject/versions" > /dev/null
}

SCHEMA_DIR="/schemas"
register_schema "game.orders.raw-value"       "$SCHEMA_DIR/OrderSubmitted.avsc"
register_schema "game.orders.validated-value" "$SCHEMA_DIR/OrderValidated.avsc"
register_schema "game.events.unit-value"      "$SCHEMA_DIR/UnitMoved.avsc"
register_schema "game.events.path-value"      "$SCHEMA_DIR/PathStatusChanged.avsc"
register_schema "game.events.region-value"    "$SCHEMA_DIR/RegionControlChanged.avsc"
register_schema "game.broadcast-value"        "$SCHEMA_DIR/WorldStateSnapshot.avsc"
register_schema "game.ring.position-value"    "$SCHEMA_DIR/RingBearerMoved.avsc"
register_schema "game.ring.detection-value"   "$SCHEMA_DIR/RingBearerDetected.avsc"
register_schema "game.dlq-value"              "$SCHEMA_DIR/DLQEntry.avsc"
register_schema "game.session-value"          "$SCHEMA_DIR/WorldStateSnapshot.avsc"

echo "Schema registration complete."
