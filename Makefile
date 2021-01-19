.PHONY: verify
verify:
	go vet ./...
	golangci-lint run ./...

.PHONY: run
run: verify
	go run ./cmd/internetz/...

.PHONY: build
build: verify
	go build -o bin/internetz ./cmd/internetz/...
	# We first try to use ping sockets, but if we have to fall back to raw sockets, set this cap on the binary and it'll request it at runtime
	# sudo setcap cap_net_raw=p ./internetz
