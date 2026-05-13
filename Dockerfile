# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git for go mod download (some modules need it)
RUN apk --no-cache add git

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o dashi .

# Stage 2: Run
FROM alpine:latest

# ca-certificates: required for HTTPS calls to Shinami and Sui RPC
# tzdata: correct timestamps in Postgres
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/dashi .

EXPOSE 8080

CMD ["./dashi"]
