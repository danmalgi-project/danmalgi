FROM golang:1.24 AS builder

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o ./webrtc ./cmd/webrtc/main.go

FROM gcr.io/distroless/static

COPY --from=builder /app/webrtc /cmd/webrtc/webrtc

ENTRYPOINT ["/cmd/webrtc/webrtc"]