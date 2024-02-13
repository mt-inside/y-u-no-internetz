lint:
	go vet ./...
	golangci-lint run ./...

run: lint
	go run ./cmd/internetz

build: lint
	go build ./cmd/internetz
	# We first try to use ping sockets, but if we have to fall back to raw sockets, set this cap on the binary and it'll request it at runtime
	# sudo setcap cap_net_raw=p ./internetz
