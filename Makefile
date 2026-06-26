.PHONY: api migration gen-route

api:
	go run ./cmd/api

migration:
	go run ./cmd/migration

gen-route:
	go run ./cmd/routegen
