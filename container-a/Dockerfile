FROM golang:1.23-alpine AS builder

# Install git for Go module downloads
RUN apk add --no-cache git

WORKDIR /app

# Copy go files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .

CMD ["./main"]