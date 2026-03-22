FROM golang:1.22-alpine AS build
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/server /server

# Run as non-root (ports >1024 need no extra capability)
USER nobody:nobody

EXPOSE 8080

ENV SERVER_ADDR=:8080

ENTRYPOINT ["/server"]
