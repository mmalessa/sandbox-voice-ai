FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/server ./cmd/server

# ---

FROM alpine:3.20

WORKDIR /app

COPY --from=builder /bin/server /bin/server
COPY web/ web/

EXPOSE 8080

ENTRYPOINT ["/bin/server"]
