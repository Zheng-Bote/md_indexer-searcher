FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build md-indexer, md-search, and md-api
RUN go build -o /app/md-indexer ./cmd/indexer
RUN go build -o /app/md-search ./cmd/md-search
RUN go build -o /app/md-api ./cmd/md-api

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/md-indexer .
COPY --from=builder /app/md-search .
COPY --from=builder /app/md-api .

EXPOSE 8080 8081
CMD ["./md-api"]
