# Stage 1: Build
# All dependencies are in vendor/ — no network access required during build.
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o dashi .

# Stage 2: Run
# No apk calls — ca-certificates are copied from the builder image.
FROM alpine:latest

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

WORKDIR /app
COPY --from=builder /app/dashi .

EXPOSE 8080

CMD ["./dashi"]
