

.PHONY: test
test:
	go fmt ./... && go vet ./... && go test -v -p=1 -cover ./...

.PHONY: run
run:
	go run ./cmd/ledgerly/main.go