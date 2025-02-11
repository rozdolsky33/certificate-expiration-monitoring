# Build stage
FROM golang:1.23 AS build

WORKDIR /app
COPY go.mod ./
COPY . .

RUN go build -o main .


FROM ubuntu:22.04

# Install necessary runtime libraries and CA certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    libc6 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /app/main /

CMD ["/main"]