# ---- build ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/notifier ./cmd/notifier

# ---- runtime ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Europe/Moscow

WORKDIR /app
COPY --from=builder /out/notifier .

USER nobody
ENTRYPOINT ["/app/notifier"]