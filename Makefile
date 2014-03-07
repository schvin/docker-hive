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
	@go test ./db
	@go test ./server
fmt:
	@go fmt .
	@go fmt ./db/
	@go fmt ./server/
clean:
	@rm -rf docker-hive
