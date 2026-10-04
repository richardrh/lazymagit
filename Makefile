.PHONY: check gofmt-check vet test-race crap-check keymapdoc-check golden-update

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

# keymapdoc exposes -check, so the ledger is verified unconditionally rather
# than through the probe that used to guard for it.
keymapdoc-check:
	go run ./internal/keymap/cmd/keymapdoc -check

# Rewrites the rendered-screen goldens under internal/ui/testdata. Review the
# diff before committing: a golden change is a visible-surface change.
golden-update:
	go test ./internal/ui -run Golden -update
