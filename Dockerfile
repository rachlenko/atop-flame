## ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG REVISION=latest
RUN go build -ldflags "-X main.appVersion=${REVISION} -s -w" -o /atop-flame .

## ── runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /atop-flame /usr/local/bin/atop-flame

ENTRYPOINT ["atop-flame"]
