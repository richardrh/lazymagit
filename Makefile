.PHONY: check gofmt-check vet test-race crap-check keymapdoc-check

check: gofmt-check vet test-race crap-check keymapdoc-check

gofmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './vendor/*'))" || \
		{ echo "gofmt is required for:"; gofmt -l $$(find . -name '*.go' -not -path './vendor/*'); exit 1; }

vet:
	go vet ./...

test-race:
	go test -race -timeout=20m ./...

crap-check:
	@profile=$$(mktemp); trap 'rm -f "$$profile"' EXIT; \
		go test -coverprofile="$$profile" ./... && \
		go run ./internal/quality/cmd/crap -coverprofile "$$profile" -threshold 12.000000001

# Once keymapdoc exposes -check, replace this probe with an unconditional call.
keymapdoc-check:
	@if grep -Eq 'flag\.(Bool|BoolVar).*"check"' internal/keymap/cmd/keymapdoc/main.go; then \
		go run ./internal/keymap/cmd/keymapdoc -check; \
	else \
		echo "keymapdoc: -check not supported; skipping"; \
	fi
