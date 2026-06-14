.PHONY: run test lint migrate-up migrate-down build docker-build

run:
	go run ./cmd/messenger-app

build:
	go build -o bin/messenger-app ./cmd/messenger-app

test:
	go test ./internal/... -v -count=1

lint:
	go vet ./...

migrate-up:
	@for f in migrations/*.up.sql; do \
		PGPASSWORD=password psql -h localhost -U user -d messenger_db -f $$f; \
	done

migrate-down:
	@for f in migrations/*.down.sql; do \
		PGPASSWORD=password psql -h localhost -U user -d messenger_db -f $$f; \
	done

docker-build:
	docker build -t messenger-app .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down
