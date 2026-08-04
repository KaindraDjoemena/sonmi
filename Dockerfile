FROM golang:1.26.5-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /sonmi-backend ./cmd/app/main.go

# Runtime stage
FROM alpine:latest

WORKDIR /root/
COPY --from=builder /sonmi-backend .

EXPOSE 8080 2222

CMD ["./sonmi-backend"]
