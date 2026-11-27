# VERTEXIA on Cloudflare Workers and Containers

Copyright (C) 2026 ItsWempo. SPDX-License-Identifier: AGPL-3.0-or-later.

This deployment keeps VERTEXIA separate from Wempo's Forum. The public API and website origin is `https://vertexia.wempos-forum.com`.

## Architecture

- The root Cloudflare Worker (`src/index.ts`) owns the custom domain and forwards HTTP and WebSocket requests to the application Container.
- The Worker uses the current Durable Object Container API directly. It does not use the deprecated `@cloudflare/containers` SDK or a second Worker under a `cloudflare/` directory.
- The Container runs the existing Go 1.25 / Fiber application from the root `Dockerfile`, listening on port 8080.
- MySQL remains the relational source of truth and is reached from the Container over TLS.
- Redis stores shared sessions, rate limits, access tokens, browser authorization codes, gameserver state, and feed coordination.
- R2 is accessed through its S3-compatible API. It is not mounted with FUSE and is not treated as a POSIX filesystem.
- Native game-server processes run on a separate UDP-capable Linux host through the remote GSM agent. The Worker and web Container are not the public UDP game-server endpoint.

## Cloudflare configuration

The root `wrangler.jsonc` defines:

- Worker name: `vertexia`
- Custom domain: `vertexia.wempos-forum.com`
- Container Durable Object class: `VertexiaContainer`
- Container image: root `Dockerfile`
- Durable Object storage: SQLite

The Container listens on port 8080. The Worker waits for `/readyz` before proxying requests, so MySQL and Redis must be reachable before the application is considered ready.

## Required resources

1. A Cloudflare R2 bucket named `vertexia-assets` (or update `R2_BUCKET_NAME` in `wrangler.jsonc`).
2. An R2 API token scoped to that bucket with Object Read and Write permissions.
3. A MySQL database reachable from Cloudflare Containers over TLS. Its certificate must validate for the configured database hostname, or a suitable CA must be supplied.
4. A Redis service reachable from the Container. Production requires shared Redis for sessions, authorization tokens, and other cross-request state.
5. Cloudflare Containers enabled for the Worker account and plan.

## Worker secrets and variables

Set secrets through Wrangler from the repository root. Never commit secret values.

Required secrets:

- `DB_HOST`
- `DB_USER`
- `DB_PASS`
- `DB_NAME`
- `ALTCHA_HMAC_KEY`
- `R2_ACCESS_KEY_ID` (or `AWS_ACCESS_KEY_ID`)
- `R2_SECRET_ACCESS_KEY` (or `AWS_SECRET_ACCESS_KEY`)
- `REDIS_URL` or `REDIS_ADDR` plus `REDIS_PASSWORD` where applicable

The non-secret R2 account ID and bucket name are declared in `wrangler.jsonc`.

Optional secrets and variables include:

- `REGISTRATION_IP_SALT`
- `GAMESERVER_API_KEY`
- `GSM_AGENT_URL`
- `GSM_AGENT_SECRET`
- `GSM_PUBLIC_HOST`
- `GSM_PUBLIC_IP`
- `NETCODE_PRIVATE_KEY`
- `NETCODE_PROTOCOL_ID`
- `TYPESAFE_API_KEY`
- `DB_TLS_CA_PEM`
- `DB_TLS_SERVER_NAME`

The Worker enables remote GSM only when the agent URL, agent secret, public game host, Redis, Netcode configuration, and gameserver API key are all present. Otherwise GSM stays disabled.

## Asset migration

Before routing users to this deployment, copy existing files into R2 while preserving their relative keys. In particular:

- `uploads/assets/**` remains under `uploads/assets/**`.
- `static/renders/meshes/**` remains under `renders/meshes/**`.
- Preserve any other keys referenced by MySQL asset records.

The application uses direct R2 object operations. Do not configure a FUSE mount or assume full local-filesystem semantics.

## Game servers and the native client

Cloudflare Worker and Container ingress is HTTP/WebSocket. It does not expose arbitrary inbound UDP for Lightyear gameplay.

Keep the native game-server process on a separate Linux host with a public UDP port range. Configure a DNS-only game hostname such as `games.vertexia.wempos-forum.com` to point to that host, then configure the remote GSM agent and `GSM_PUBLIC_HOST`.

The native client uses a short-lived Netcode connect token and a one-time browser authorization code. It exchanges that code at `/api/v1/auth/exchange` for a client-scoped bearer token. The long-lived gameserver API key and Netcode private key must never be embedded in the distributed client.

The client repository is maintained separately: [Wempo-s-Silly-Projects/RaveEngine-Game](https://github.com/Wempo-s-Silly-Projects/RaveEngine-Game).

## Deploy

Run these commands from the repository root:

```sh
pnpm install
pnpm run types
pnpm run check
pnpm run deploy
```

No production deployment is implied by this document. Configure the database, Redis, R2, and required secrets before deploying.

## Post-deployment verification

- `GET /healthz` reports that the Go process is serving HTTP.
- `GET /readyz` confirms MySQL and Redis are reachable.
- Verify login, registration, logout, and session continuity.
- Verify MySQL-backed profile, game, avatar, and other platform operations.
- Verify `/ws/feed` WebSocket upgrades and cross-request feed updates.
- Verify R2 upload, immediate read-after-write, deletion, and persistence across Container restarts.
- Verify the native client receives a one-time code and connect token, exchanges the code, and joins a gameserver through the separate UDP hostname.

## Operational limits

- The Worker currently routes to a named Container Durable Object. Plan capacity and availability around that deployment shape.
- R2 object storage is not a local filesystem. Application paths must use the storage abstraction and object semantics.
- Game-server UDP traffic remains outside the Worker and web Container.
- Do not enable production GSM until the remote agent, Redis coordination, server binary, UDP firewall, and client join flow have all been verified.
