.PHONY: run-all
run-all:
	@echo "=== go build ./... ===" && \
	go build ./... 2>&1; \
	echo "=== EXIT: $$? ===" && \
	echo "" && \
	echo "=== go vet ./... ===" && \
	go vet ./... 2>&1; \
	echo "=== EXIT: $$? ===" && \
	echo "" && \
	echo "=== go test orchestrator ===" && \
	go test -race -count=1 ./internal/builtin/orchestrator/... 2>&1 | tail -5; \
	echo "=== EXIT: $${PIPESTATUS[0]} ===" && \
	echo "" && \
	echo "=== go test pty ===" && \
	go test -race -count=1 ./internal/builtin/pty/... 2>&1 | tail -5; \
	echo "=== EXIT: $${PIPESTATUS[0]} ==="
