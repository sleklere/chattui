.PHONY: lint run-server run-client migrate migrate-down migrate-status check-e2e check-protocol check-room check-dm

lint:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	golangci-lint run ./...

run-server:
	go run ./cmd/server

run-client:
	go run ./cmd/client

# Supply a new absolute CHECK_RESULTS path for each invocation.
check-e2e check-protocol check-room check-dm:
	@test -n "$(CHECK_RESULTS)" || { echo 'set CHECK_RESULTS to a fresh absolute directory' >&2; exit 1; }
	go run -mod=readonly ./cmd/check --repository "$(CURDIR)" --results "$(CHECK_RESULTS)" $(if $(filter check-protocol,$@),--mode protocol,$(if $(filter check-room,$@),--mode compare --profile room,$(if $(filter check-dm,$@),--mode compare --profile dm,)))


DB_URL ?= $(shell grep DB_URL .env | cut -d '=' -f2-)

migrate:
	goose -dir ./cmd/server/migrations postgres "$(DB_URL)" up

migrate-down:
	goose -dir ./cmd/server/migrations postgres "$(DB_URL)" down

migrate-status:
	goose -dir ./cmd/server/migrations postgres "$(DB_URL)" status

