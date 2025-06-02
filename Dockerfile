FROM golang:1.23.9-alpine AS builder

WORKDIR /app
COPY . .

RUN go build -o bin/s3-proxy ./cmd/s3-proxy/main.go



FROM alpine:3.21.3

WORKDIR /app
COPY --from=builder /app/bin/s3-proxy /app/s3-proxy

EXPOSE 8080

CMD ["/app/s3-proxy", "s3-proxy"]
