FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o boxcheckr ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

RUN addgroup -g 1000 boxcheckr && \
    adduser -u 1000 -G boxcheckr -h /app -D boxcheckr

WORKDIR /app

COPY --from=builder /app/boxcheckr .
COPY --from=builder /app/web ./web

RUN mkdir -p /data && chown boxcheckr:boxcheckr /data

EXPOSE 8080

ENV DATABASE_PATH=/data/boxcheckr.db

VOLUME ["/data"]

USER boxcheckr

CMD ["./boxcheckr"]
