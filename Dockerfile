FROM golang:1.23 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o book-club-bot ./cmd/main.go

FROM debian:slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=builder /app/book-club-bot .
COPY --from=builder /app/config ./config
COPY --from=builder /app/message ./message
COPY --from=builder /app/assets ./assets
RUN mkdir ./db
CMD [ "./book-club-bot" ]
