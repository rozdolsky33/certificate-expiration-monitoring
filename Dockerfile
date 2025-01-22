FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN go build -o main .

FROM debian:bullseye-slim

COPY --from=builder /app/main /main

RUN useradd -m appuser
USER appuser

ENTRYPOINT ["/main"]