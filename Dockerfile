# Multi-stage build for the SmellyFeet frontend (stdlib-only Go app).
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

# Copy everything (templates are embedded via //go:embed) and build a static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-w -s' -o smellyfeet .

# Minimal runtime image.
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /app/smellyfeet /app/smellyfeet

# Non-root (nobody) for the scratch image.
USER 65534:65534
WORKDIR /app

EXPOSE 3000
CMD ["./smellyfeet"]
