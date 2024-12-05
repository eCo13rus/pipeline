FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o pipeline main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/pipeline .
CMD ["./pipeline"]