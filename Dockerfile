# Build stage
FROM golang:1.26-alpine AS build

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

# Install goose for migrations
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/server ./cmd/server

# Runtime stage
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=build /bin/server /usr/local/bin/server
COPY --from=build /go/bin/goose /usr/local/bin/goose
COPY cmd/server/migrations /migrations

CMD ["server"]
