FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/blog-server ./cmd/blog-server

FROM alpine:3.22
RUN addgroup -S blog && adduser -S blog -G blog && mkdir -p /data/uploads && chown -R blog:blog /data
COPY --from=build /out/blog-server /usr/local/bin/blog-server
USER blog
EXPOSE 8081
ENTRYPOINT ["/usr/local/bin/blog-server"]
