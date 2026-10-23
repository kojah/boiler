GO ?= go
RUN ?= $(shell command -v agent-test 2>/dev/null)
CONFIG := examples/immutable/generate.boiler
.PHONY: build generate generated-check test vet format-check check-source
build: generate format-check check-source
	$(RUN) $(GO) build ./...
generate:
	$(RUN) $(GO) run ./cmd/boiler generate --config $(CONFIG)
generated-check:
	$(RUN) $(GO) run ./cmd/boiler generate --check --config $(CONFIG)
test: generate
	$(RUN) $(GO) test -p=1 -parallel=4 ./...
vet: generate
	$(RUN) $(GO) vet ./...
format-check:
	@test -z "$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go'))"
check-source:
	python3 scripts/check-source.py
