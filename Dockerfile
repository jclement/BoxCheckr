FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build -o boxcheckr ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/boxcheckr .
COPY --from=builder /app/web ./web

EXPOSE 8080

ENV DATABASE_PATH=/data/boxcheckr.db

VOLUME ["/data"]

CMD ["./boxcheckr"]
