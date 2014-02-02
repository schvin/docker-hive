all:
	@go get -d -v ./...
	@go build .
build:
	@go build .
fmt:
	@go fmt ./...
clean:
	@rm -rf docker-cluster
