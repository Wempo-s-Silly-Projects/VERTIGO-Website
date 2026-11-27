# VERTEXIA on Cloudflare Workers

Copyright (C) 2026 ItsWempo. SPDX-License-Identifier: AGPL-3.0-or-later.

This deployment keeps the VERTEXIA application separate from Wempo's Forum. The public hostname is `https://vertexia.wempos-forum.com`; the Worker is named `vertexia`.

## Architecture

- A Cloudflare Worker owns the public hostname and forwards HTTP and WebSocket requests.
- A singleton Cloudflare Container runs the existing Go 1.25 / Fiber application. It is deliberately a singleton because the current application keeps its live-feed WebSocket hub and Fiber sessions in process memory.
- The existing MySQL database remains the source of truth and is reached from the container over the network.
- R2 is mounted into the container for uploaded assets and generated render files.
- Game-server process management is not enabled inside the web container. The current launcher starts native game-server processes and expects publicly reachable game ports; that service needs a separate Linux game-server host and network design.

## Required Cloudflare resources

Cloudflare Containers are available only on the **Workers Paid** plan. The connected Cloudflare account currently lacks Container access, so deployment is blocked until that plan is enabled. The Workers Paid subscription is separate from the website's Free Website zone plan.

1. An R2 bucket named `vertexia-assets` (or change `R2_BUCKET_NAME` in `wrangler.jsonc`).
2. An R2 API token scoped to that bucket with Object Read and Write permissions.
3. A MySQL database reachable from Cloudflare Containers. The Worker enables MySQL TLS with certificate verification. The database must allow connections from Cloudflare's container egress and present a certificate valid for its hostname.
4. Cloudflare Containers enabled for the account.

The R2 mount uses the pinned `tigrisfs` release in the Dockerfile. FUSE access is not a full POSIX filesystem and is not equivalent to local SSD storage. Exercise upload, immediate read-after-write, mesh preview, delete, and restart behavior before accepting production traffic.

## Worker secrets

Set these with Wrangler. Never commit secret values:

```sh
pnpm install
pnpm run types

pnpm exec wrangler secret put DB_HOST
pnpm exec wrangler secret put DB_USER
pnpm exec wrangler secret put DB_PASS
pnpm exec wrangler secret put DB_NAME
pnpm exec wrangler secret put ALTCHA_HMAC_KEY
pnpm exec wrangler secret put AWS_ACCESS_KEY_ID
pnpm exec wrangler secret put AWS_SECRET_ACCESS_KEY

# Optional integrations
pnpm exec wrangler secret put REGISTRATION_IP_SALT
pnpm exec wrangler secret put GAMESERVER_API_KEY
pnpm exec wrangler secret put TYPESAFE_API_KEY
```

Set `DB_PORT` in `wrangler.jsonc` or as a Worker variable if the database does not use port 3306. The R2 account ID and bucket name are non-secret variables already declared in the Wrangler config.

The database must be reachable from the container. A private database that only accepts connections from a private network needs a supported Cloudflare network path before this deployment can work.

## Asset migration

Before routing users to this deployment, copy existing files from the current VERTEXIA host into the matching R2 keys:

- `uploads/assets/**` stays under `uploads/assets/**`.
- `static/renders/meshes/**` stays under `renders/meshes/**`.

The MySQL asset rows store relative file paths, so preserving the keys is required. Do not point production traffic at an empty bucket if existing assets are expected to remain available.

## Game servers and the RaveEngine client

`GSM_ENABLED=false` is intentional. The existing Go process launcher defaults to a Windows executable path and launches native game-server processes with per-server network ports. Those ports are not made publicly reachable merely by putting the web application behind a Worker. Cloudflare Containers receive end-user requests through the Worker and do not expose arbitrary inbound UDP to a Container. RaveEngine uses Lightyear over UDP for real-time gameplay, so the game-server process must run on a separate UDP-capable host.

The RaveEngine client is maintained separately in [Wempo-s-Silly-Projects/RaveEngine-Game](https://github.com/Wempo-s-Silly-Projects/RaveEngine-Game). Its HTTP API default is `https://vertexia.wempos-forum.com`; its join URI must receive the actual game-server host and UDP port, not the Worker hostname. Configure a separate DNS-only game hostname (for example, `games.vertexia.wempos-forum.com`) to resolve directly to the dedicated game-server host. Open the configured UDP port range on that host and set `GSM_PUBLIC_HOST` to that hostname.

Cloudflare Spectrum can proxy custom UDP applications, but current Cloudflare plan documentation lists arbitrary custom UDP as an Enterprise add-on. A regular Workers Paid subscription does not make the Worker or Container an inbound UDP game-server endpoint. See [Cloudflare Spectrum protocols per plan](https://developers.cloudflare.com/spectrum/protocols-per-plan/) and [Container networking limitations](https://developers.cloudflare.com/containers/platform-details/architecture/).

Keep game-server processes on a separate Linux host with the appropriate public networking, then implement and verify a dedicated manager/API integration before enabling game-server creation in production.

## Deploy

A Docker-compatible engine must be running locally because Wrangler builds and uploads the Container image during deployment.

```sh
pnpm install
pnpm run types
pnpm run check
pnpm run deploy
```

The first Container deployment can take several minutes to provision. After deployment, verify the Container is healthy, then test:

- Login, registration, logout, and session continuity.
- MySQL-backed profile, forum, shop, clan, game, and avatar operations.
- `/ws/feed` WebSocket upgrades and live updates.
- Asset upload, read, moderation, deletion, and generated mesh previews.
- Container restart and R2 persistence.

## Operational limitations to close before full production

- Fiber's default session store is in-memory. A container restart invalidates active sessions; use a shared durable session store before relying on long-lived sessions.
- The live feed WebSocket hub is process-local. Keep one application container until the hub is moved to shared coordination.
- The game-server manager requires a separate host and public game-port networking.
- R2 FUSE has different performance and filesystem semantics from a local disk. Validate the actual asset workload and migrate to direct R2 API storage if the application needs stronger filesystem guarantees.
