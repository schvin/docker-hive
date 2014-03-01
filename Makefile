all:
	@go build .
	@go get -d ./...

deps:
	@go get -d ./...

build:
	@go build .
benchmark:
	@go test -bench=. ./...
test:
	@go test ./...
fmt:
	@go fmt ./...
clean:
	@rm -rf docker-hive
