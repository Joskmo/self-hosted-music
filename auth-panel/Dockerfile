FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-panel ./cmd/auth-panel

FROM alpine:3.19
RUN apk add --no-cache unzip
WORKDIR /app
COPY --from=builder /auth-panel .
ENV PORT=3000
EXPOSE 3000
CMD ["./auth-panel"]
