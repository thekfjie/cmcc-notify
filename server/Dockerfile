FROM node:24-bookworm-slim AS web-build
WORKDIR /src/server/web
COPY server/web/package.json server/web/package-lock.json ./
RUN npm ci
COPY server/web ./
RUN npm run build

FROM golang:1.26.0-bookworm AS build
ARG VERSION=dev
WORKDIR /src
COPY cmcc/go.mod cmcc/go.sum ./cmcc/
COPY server/go.mod server/go.sum ./server/
RUN cd server && GOWORK=off go mod download
COPY cmcc ./cmcc
COPY server ./server
COPY --from=web-build /src/server/internal/httpapi/web ./server/internal/httpapi/web
WORKDIR /src/server
RUN GOWORK=off go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/cmcc-notify ./cmd/cmcc-notify

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --home /nonexistent --shell /usr/sbin/nologin cmcc-notify \
    && install -d -o cmcc-notify -g cmcc-notify -m 0700 /var/lib/cmcc-notify
COPY --from=build /out/cmcc-notify /usr/local/bin/cmcc-notify
USER cmcc-notify
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cmcc-notify"]
CMD ["-config", "/etc/cmcc-notify/config.yaml"]
