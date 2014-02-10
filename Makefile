all:
	@go get -d -v ./...
	@go build .
build:
	@go build .
benchmark:
	@go test -bench=. ./...
test:
	@go test ./...
fmt:
	@go fmt ./...
clean:
	@rm -rf docker-cluster
