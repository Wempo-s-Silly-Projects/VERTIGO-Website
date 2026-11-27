# Copyright (C) 2026 ItsWempo
# SPDX-License-Identifier: AGPL-3.0-or-later

FROM golang:1.25.0-alpine3.22 AS build
WORKDIR /src
RUN apk add --no-cache git
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/vertexia .

FROM alpine:3.22
ARG TIGRISFS_VERSION=1.2.1
RUN apk add --no-cache ca-certificates curl fuse tar util-linux \
    && curl -fL "https://github.com/tigrisdata/tigrisfs/releases/download/v${TIGRISFS_VERSION}/tigrisfs_${TIGRISFS_VERSION}_linux_amd64.tar.gz" -o /tmp/tigrisfs.tar.gz \
    && tar -xzf /tmp/tigrisfs.tar.gz -C /usr/local/bin/ \
    && rm /tmp/tigrisfs.tar.gz \
    && chmod 0755 /usr/local/bin/tigrisfs

WORKDIR /app
COPY --from=build /out/vertexia /usr/local/bin/vertexia
COPY --from=build /src/views ./views
COPY --from=build /src/static ./static
COPY --from=build /src/assets ./assets
COPY container-entrypoint.sh /usr/local/bin/container-entrypoint
RUN chmod 0755 /usr/local/bin/container-entrypoint \
    && mkdir -p /mnt/r2 /app/uploads /app/static/renders
ENV PORT=3000 \
    ADDR=:3000 \
    RUN_WITH_DATABASE=true \
    GSM_ENABLED=false \
    SESSION_SECURE=true \
    SESSION_SAMESITE=Lax
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/container-entrypoint"]
