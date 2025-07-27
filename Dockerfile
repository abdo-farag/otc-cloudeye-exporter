# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o otc-cloudeye-exporter ./cmd/otc_cloudeye_exporter

# Final stage: Distroless (non-root)
FROM gcr.io/distroless/static:nonroot

WORKDIR /app
COPY --from=builder /app/otc-cloudeye-exporter /app/
COPY --from=builder /app/*.yml /app/

USER nonroot

EXPOSE 9098 9099

ENTRYPOINT ["/app/otc-cloudeye-exporter", "-config", "/app/clouds.yml"]
