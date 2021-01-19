.PHONY: verify
verify:
	go vet ./...
	golangci-lint run ./...

.PHONY: run
run: verify
	go run ./cmd/internetz/...

.PHONY: build
build: verify
	go build -o internetz ./cmd/internetz/...
	sudo setcap cap_net_raw=p ./internetz