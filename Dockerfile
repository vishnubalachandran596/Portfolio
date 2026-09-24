# Local and single-host image for cmd/server. Vercel still uses api/index.go
# and does not build this file.
FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/portfolio ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates su-exec \
    && adduser -D -H -u 65532 app
COPY --from=build /out/portfolio /usr/local/bin/portfolio
COPY docker/entrypoint.sh /entrypoint.sh
# A Windows checkout can leave CRLF in the script. Strip it in the image
# so the entrypoint runs on Docker Desktop for Windows and for Mac.
RUN sed -i 's/\r$//' /entrypoint.sh && chmod 755 /entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
