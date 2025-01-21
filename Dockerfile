FROM golang:1.23 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o book-club-bot ./cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/book-club-bot .
COPY --from=builder /app/config ./config
COPY --from=builder /app/message ./message
COPY --from=builder /app/assets ./assets
RUN mkdir ./db
CMD [ "./book-club-bot" ]
