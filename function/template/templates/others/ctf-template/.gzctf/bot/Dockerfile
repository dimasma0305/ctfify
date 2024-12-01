
FROM golang:1.22 AS builder

WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o bot main.go

FROM ubuntu:noble

RUN apt-get update && apt-get install -y ca-certificates

COPY --from=builder /app/bot /app/bot
WORKDIR /app

CMD ["./bot"]
