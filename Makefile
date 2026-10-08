
build:
	go build -o bin/md-indexer ./cmd/indexer

search:
	go build -o bin/md-search ./cmd/md-search

api:
	go build -o bin/md-api ./cmd/md-api

api-run: api
	./bin/md-api -port 8081

test:
	go test -v ./...
