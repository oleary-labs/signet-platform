FROM golang:1.25-alpine AS build
WORKDIR /src

# Dependencies first, so a source-only change does not re-download the module
# cache on every build.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed   ./cmd/seed

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 10001 signet
WORKDIR /app
COPY --from=build /out/server /out/seed /usr/local/bin/
# Local uploads land here when IN_PRODUCTION is false.
RUN mkdir -p /app/uploads /app/logs && chown -R signet /app
USER signet
EXPOSE 8088
CMD ["server"]
