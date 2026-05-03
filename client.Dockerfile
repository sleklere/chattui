FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/client ./cmd/client

FROM alpine:3.21
COPY --from=build /bin/client /usr/local/bin/client
ENTRYPOINT ["client"]
