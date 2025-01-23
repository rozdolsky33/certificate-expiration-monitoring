# Build stage
FROM golang:1.23 AS build

WORKDIR /app
COPY go.mod ./
COPY . .

RUN go build -o main .


FROM ubuntu:22.04

RUN apt-get update && apt-get install -y libc6

COPY --from=build /app/main /

CMD ["/main"]