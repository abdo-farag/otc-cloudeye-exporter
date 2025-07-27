# syntax=docker/dockerfile:1.4

FROM golang:1.24-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o otc-cloudeye-exporter ./cmd/otc-cloudeye-exporter

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=builder /app/otc-cloudeye-exporter /app/
COPY --from=builder /app/*.yml /app/

USER nonroot

EXPOSE 9098 9099

ENTRYPOINT ["/app/otc-cloudeye-exporter", "-config", "/app/clouds.yml"]