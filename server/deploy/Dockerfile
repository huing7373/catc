FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /catserver ./cmd/cat

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app

COPY --from=builder /catserver ./catserver
COPY config ./config

EXPOSE 8080
ENTRYPOINT ["./catserver"]
CMD ["-config", "config/production.toml"]
