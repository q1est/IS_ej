FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
COPY --from=builder /out/api /api
COPY --from=builder /out/healthcheck /healthcheck
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
EXPOSE 8080
USER app:app
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/healthcheck"]
ENTRYPOINT ["/api"]
