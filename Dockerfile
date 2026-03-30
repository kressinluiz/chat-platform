FROM golang:1.26-alpine

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
COPY frontend ./frontend

RUN go build -o main .

CMD ["./main"]
