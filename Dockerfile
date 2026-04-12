FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates \
    && go install github.com/swaggo/swag/v2/cmd/swag@latest

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN swag init -g cmd/service/main.go -o docs --parseDependency --parseInternal
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app ./cmd/service

# ---- Runtime ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup

COPY --from=builder /app /app

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app"]
