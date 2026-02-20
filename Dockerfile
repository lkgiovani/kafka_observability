FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG CMD
RUN go build -o /bin/app ./cmd/${CMD}

FROM alpine:latest

COPY --from=builder /bin/app /bin/app

ENTRYPOINT ["/bin/app"]
