FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
COPY frontend ./frontend

RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/.env ./.env
COPY --from=builder /app/frontend ./frontend
COPY --from=builder /app/migrations ./migrations

CMD ["./main"]
