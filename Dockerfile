# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /aegis ./cmd/aegis

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /aegis /aegis
COPY config.example.yaml /etc/aegis/config.yaml

EXPOSE 8080 9090
ENTRYPOINT ["/aegis"]
CMD ["serve", "--config", "/etc/aegis/config.yaml"]
