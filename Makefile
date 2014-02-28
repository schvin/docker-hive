all:
	@go build .
	@go get -d ./...

depends:
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
