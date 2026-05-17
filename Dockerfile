FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /knowing ./cmd/knowing

FROM alpine:3.21

RUN apk add --no-cache ca-certificates git

COPY --from=builder /knowing /usr/local/bin/knowing

VOLUME /data
WORKDIR /data

EXPOSE 8080

ENTRYPOINT ["knowing"]
CMD ["serve", "-db", "/data/knowing.db", "-addr", ":8080"]
