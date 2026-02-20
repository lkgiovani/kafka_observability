.PHONY: up down producer consumer tidy

# Sobe o Kafka via Docker
up:
	docker-compose up -d
	@echo "⏳ Aguardando Kafka iniciar..."
	@sleep 5
	@echo "✅ Kafka pronto em localhost:9092"

# Para e remove os containers
down:
	docker-compose down

# Instala dependências
tidy:
	go mod tidy

# Roda o producer
producer:
	go run ./cmd/producer/main.go

# Roda o consumer
consumer:
	go run ./cmd/consumer/main.go

# Roda producer e consumer em paralelo (requer tmux ou dois terminais)
run-all:
	@echo "Abra dois terminais e execute:"
	@echo "  Terminal 1: make producer"
	@echo "  Terminal 2: make consumer"