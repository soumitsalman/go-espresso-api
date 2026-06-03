# Stage 1: Builder with protobuf tools
FROM golang:1.26-alpine AS builder

# Set working directory
WORKDIR /app
COPY . .

RUN go mod download
RUN GOOS=linux go build -o espressoapi .

# Stage 2: Minimal runtime image
FROM alpine:latest
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/espressoapi .

ENV PORT=8080
ENV GIN_MODE=release

EXPOSE 8080

ENTRYPOINT ["/app/espressoapi"]