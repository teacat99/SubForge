FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /subforge ./cmd/server/

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /subforge /usr/local/bin/subforge
RUN mkdir -p /data
EXPOSE 8080
ENTRYPOINT ["subforge"]
CMD ["-port", "8080", "-db", "/data/subforge.db"]
