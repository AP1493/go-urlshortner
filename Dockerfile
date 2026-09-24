# ---- build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Dependencies first so the module cache layer survives source edits.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary: no libc at runtime, so the final image can stay minimal.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd

# ---- runtime stage ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget \
    && adduser -D -u 10001 appuser

WORKDIR /app

COPY --from=builder /out/server ./server
COPY views ./views

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

CMD ["./server"]
