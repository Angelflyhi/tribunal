FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tribunal ./cmd/tribunal

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/tribunal .
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["./tribunal"]
