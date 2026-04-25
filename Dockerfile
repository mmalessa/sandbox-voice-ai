FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /bin/server ./cmd/server

# ---

FROM python:3.11-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg \
    && rm -rf /var/lib/apt/lists/*

COPY stt/requirements.txt stt/requirements.txt
RUN pip install --no-cache-dir -r stt/requirements.txt

COPY --from=builder /bin/server /bin/server
COPY web/ web/
COPY stt/wrapper.py stt/wrapper.py

EXPOSE 8080

ENTRYPOINT ["/bin/server"]
