.PHONY: up down test build logs clean

# Start the entire system
up:
	docker compose up --build -d

# Stop the system
down:
	docker compose down -v

# Run unit tests (no Docker or Kafka required)
test:
	cd option-b && go test ./tests/... -v -race

# Run tests with coverage
test-cover:
	cd option-b && go test ./tests/... -coverprofile=coverage.out && go tool cover -html=coverage.out

# Build the Go binary locally
build:
	cd option-b && go build ./...

# Build the Docker image
build-docker:
	docker compose build go-1

# View logs from all Go instances
logs:
	docker compose logs -f go-1 go-2 go-3

# Clean up all containers and volumes
clean:
	docker compose down -v --remove-orphans
	rm -rf option-b/coverage.out

# Demo: stop go-2 to observe consumer group rebalance (Section 34 Scenario 3)
demo-kill-go2:
	docker compose stop go-2

demo-start-go2:
	docker compose start go-2

# Show Kafka topics
topics:
	docker compose exec kafka-1 kafka-topics --bootstrap-server kafka-1:9092 --describe

# Inspect game.broadcast topic
inspect-broadcast:
	docker compose exec kafka-1 kafka-console-consumer \
		--bootstrap-server kafka-1:9092 \
		--topic game.broadcast \
		--from-beginning \
		--max-messages 20
