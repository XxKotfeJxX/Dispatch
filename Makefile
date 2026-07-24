.PHONY: test test-race test-ai lint compose-up compose-down

test:
	go test ./...
	pnpm --dir web test

test-race:
	go test -race ./...

test-ai:
	go test -tags=manual_ai -run TestGeminiManual ./internal/ai/gemini

lint:
	gofmt -w cmd internal
	go vet ./...
	pnpm --dir web lint
	pnpm --dir web typecheck

compose-up:
	docker compose up --build

compose-down:
	docker compose down
