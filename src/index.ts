// Copyright (C) 2026 ItsWempo
// SPDX-License-Identifier: AGPL-3.0-or-later

import { DurableObject } from "cloudflare:workers";

const APP_PORT = 8080;
const INACTIVITY_TIMEOUT_MS = 30 * 60 * 1000;
const STARTUP_ATTEMPTS = 90;
const STARTUP_RETRY_MS = 500;

type VertexiaRuntimeEnv = Env & {
  DB_HOST?: string;
  DB_PORT?: string;
  DB_USER?: string;
  DB_PASS?: string;
  DB_NAME?: string;
  DB_TLS_CA_PEM?: string;
  DB_TLS_SERVER_NAME?: string;
  DB_TIMEOUT?: string;
  ALTCHA_HMAC_KEY?: string;
  REGISTRATION_IP_SALT?: string;
  AWS_ACCESS_KEY_ID?: string;
  AWS_SECRET_ACCESS_KEY?: string;
  R2_ACCOUNT_ID?: string;
  R2_BUCKET_NAME?: string;
  R2_BUCKET?: string;
  R2_ACCESS_KEY_ID?: string;
  R2_SECRET_ACCESS_KEY?: string;
  UPLOAD_MAX_BYTES?: string;
  STORAGE_DIR?: string;
  SITELOCK_PASSWORD?: string;
  GSM_PORT_RANGE?: string;
  GSM_SERVER_BIN?: string;
  GSM_SERVER_WORKDIR?: string;
  GSM_SERVER_MAP?: string;
  GSM_SERVER_LOGS_DIR?: string;
  TYPESAFE_API_KEY?: string;
  GAMESERVER_API_KEY?: string;
  GSM_AGENT_URL?: string;
  GSM_AGENT_SECRET?: string;
  GSM_AGENT_TIMEOUT?: string;
  GSM_MAX_PLAYERS_DEFAULT?: string;
  GSM_MIN_PLAYERS_DEFAULT?: string;
  GSM_IDLE_TIMEOUT?: string;
  GSM_REAP_INTERVAL?: string;
  GSM_HEARTBEAT_TIMEOUT?: string;
  GSM_MAX_SERVERS?: string;
  GSM_READY_TIMEOUT?: string;
  GSM_PUBLIC_HOST?: string;
  GSM_PORT_MIN?: string;
  GSM_PORT_MAX?: string;
  REDIS_ADDR?: string;
  REDIS_URL?: string;
  REDIS_TLS?: string;
  NETCODE_PRIVATE_KEY?: string;
  NETCODE_PROTOCOL_ID?: string;
  NETCODE_TOKEN_TTL?: string;
  CLIENT_TOKEN_TTL?: string;
  STUDIO_TOKEN_TTL?: string;
  GSM_PUBLIC_IP?: string;
  R2_ENDPOINT?: string;
  R2_PUBLIC_BASE_URL?: string;
  REDIS_PASSWORD?: string;
  REDIS_DB?: string;
  TRUSTED_PROXIES?: string;
  TYPESAFE_BASE_URL?: string;
  TYPESAFE_MODEL?: string;
  FILTER_TIMEOUT?: string;
  SITELOCK_ENABLED?: string;
  LIVETIMER_ENABLED?: string;
  LIVETIMER_DURATION?: string;
};

export class VertexiaContainer extends DurableObject<VertexiaRuntimeEnv> {
  private starting: Promise<void> | undefined;

  constructor(ctx: DurableObjectState, env: VertexiaRuntimeEnv) {
    super(ctx, env);
    if (ctx.container?.running) {
      void ctx.blockConcurrencyWhile(() =>
        ctx.container!.setInactivityTimeout(INACTIVITY_TIMEOUT_MS),
      );
    }
  }

  async fetch(request: Request): Promise<Response> {
    try {
      this.starting ??= this.startAndWaitForPort().finally(() => {
        this.starting = undefined;
      });
      await this.starting;

      const url = new URL(request.url);
      url.protocol = "http:";
      url.host = "vertexia-container";
      const headers = new Headers(request.headers);
      for (const name of [...headers.keys()]) {
        if (name.startsWith("x-forwarded-")) headers.delete(name);
      }
      headers.delete("forwarded");
      headers.delete("x-real-ip");
      headers.delete("host");
      const forwarded = new Request(url, { ...request, headers });

      const connectingIP = request.headers.get("CF-Connecting-IP");
      if (connectingIP) {
        forwarded.headers.set("X-Forwarded-For", connectingIP);
      }
      forwarded.headers.set(
        "X-Forwarded-Proto",
        new URL(request.url).protocol.replace(":", ""),
      );
      forwarded.headers.set("X-Forwarded-Host", "vertexia.wempos-forum.com");

      // Return the container response directly so WebSocket upgrades remain intact.
      return this.ctx.container!.getTcpPort(APP_PORT).fetch(forwarded);
    } catch (error) {
      console.error(
        JSON.stringify({
          message: "VERTEXIA container request failed",
          error: error instanceof Error ? error.message : String(error),
        }),
      );
      return new Response("VERTEXIA is temporarily unavailable.", {
        status: 503,
        headers: {
          "content-type": "text/plain; charset=utf-8",
          "retry-after": "10",
        },
      });
    }
  }

  private async startAndWaitForPort(): Promise<void> {
    const container = this.ctx.container;
    if (!container) {
      throw new Error("VERTEXIA container binding is unavailable");
    }

    const required: Array<[string, string | undefined]> = [
      ["DB_HOST", this.env.DB_HOST],
      ["DB_USER", this.env.DB_USER],
      ["DB_PASS", this.env.DB_PASS],
      ["DB_NAME", this.env.DB_NAME],
      ["ALTCHA_HMAC_KEY", this.env.ALTCHA_HMAC_KEY],
      ["R2_ACCESS_KEY_ID", this.env.R2_ACCESS_KEY_ID || this.env.AWS_ACCESS_KEY_ID],
      ["R2_SECRET_ACCESS_KEY", this.env.R2_SECRET_ACCESS_KEY || this.env.AWS_SECRET_ACCESS_KEY],
    ];
    const missing = required.filter(([, value]) => !value).map(([name]) => name);
    if (missing.length > 0) {
      throw new Error(`Missing required Worker secrets: ${missing.join(", ")}`);
    }
    if (!this.env.R2_ACCOUNT_ID || !(this.env.R2_BUCKET_NAME || this.env.R2_BUCKET)) {
      throw new Error("R2_ACCOUNT_ID and R2_BUCKET_NAME must be configured");
    }
    if (!this.env.REDIS_URL && !this.env.REDIS_ADDR) {
      throw new Error("REDIS_URL or REDIS_ADDR must be configured for durable production sessions");
    }

    const gsmConfigured = Boolean(
      this.env.GSM_AGENT_URL &&
      this.env.GSM_AGENT_SECRET &&
      this.env.GSM_PUBLIC_HOST &&
      (this.env.REDIS_URL || this.env.REDIS_ADDR) &&
      this.env.NETCODE_PRIVATE_KEY &&
      this.env.NETCODE_PROTOCOL_ID &&
      this.env.GAMESERVER_API_KEY,
    );

    if (!container.running) {
      container.start({
        image: container.images.base,
        instance: "standard-1",
        enableInternet: true,
        env: {
          PORT: String(APP_PORT),
          ADDR: `:${APP_PORT}`,
          APP_ENV: "production",
          PUBLIC_BASE_URL: "https://vertexia.wempos-forum.com",
          RUN_WITH_DATABASE: "true",
          DB_HOST: this.env.DB_HOST!,
          DB_PORT: this.env.DB_PORT || "3306",
          DB_USER: this.env.DB_USER!,
          DB_PASS: this.env.DB_PASS!,
          DB_NAME: this.env.DB_NAME!,
          DB_TLS: "true",
          DB_TLS_CA_PEM: this.env.DB_TLS_CA_PEM || "",
          DB_TLS_SERVER_NAME: this.env.DB_TLS_SERVER_NAME || "",
          DB_TIMEOUT: this.env.DB_TIMEOUT || "10s",
          DB_MAX_OPEN_CONNS: "20",
          DB_MAX_IDLE_CONNS: "5",
          DB_CONN_MAX_LIFETIME: "5m",
          DB_CONN_MAX_IDLE_TIME: "5m",
          GSM_ENABLED: gsmConfigured ? "true" : "false",
          GSM_AGENT_URL: this.env.GSM_AGENT_URL || "",
          GSM_AGENT_SECRET: this.env.GSM_AGENT_SECRET || "",
          GSM_AGENT_TIMEOUT: this.env.GSM_AGENT_TIMEOUT || "10s",
          GSM_PUBLIC_HOST: this.env.GSM_PUBLIC_HOST || "",
          GSM_PORT_MIN: this.env.GSM_PORT_MIN || "5100",
          GSM_PORT_MAX: this.env.GSM_PORT_MAX || "5250",
          GSM_PORT_RANGE: this.env.GSM_PORT_RANGE || "",
          GSM_SERVER_BIN: this.env.GSM_SERVER_BIN || "",
          GSM_SERVER_WORKDIR: this.env.GSM_SERVER_WORKDIR || "",
          GSM_SERVER_MAP: this.env.GSM_SERVER_MAP || "assets/maps/default.vrtx",
          GSM_SERVER_LOGS_DIR: this.env.GSM_SERVER_LOGS_DIR || "./logs/gsm",
          GSM_MAX_PLAYERS_DEFAULT: this.env.GSM_MAX_PLAYERS_DEFAULT || "100",
          GSM_MIN_PLAYERS_DEFAULT: this.env.GSM_MIN_PLAYERS_DEFAULT || "2",
          GSM_IDLE_TIMEOUT: this.env.GSM_IDLE_TIMEOUT || "5m",
          GSM_REAP_INTERVAL: this.env.GSM_REAP_INTERVAL || "15s",
          GSM_HEARTBEAT_TIMEOUT: this.env.GSM_HEARTBEAT_TIMEOUT || "90s",
          GSM_MAX_SERVERS: this.env.GSM_MAX_SERVERS || "",
          GSM_READY_TIMEOUT: this.env.GSM_READY_TIMEOUT || "30s",
          REDIS_REQUIRED: "true",
          REDIS_URL: this.env.REDIS_URL || "",
          REDIS_ADDR: this.env.REDIS_ADDR || "",
          REDIS_TLS: this.env.REDIS_TLS || "false",
          REDIS_PASSWORD: this.env.REDIS_PASSWORD || "",
          REDIS_DB: this.env.REDIS_DB || "0",
          SESSION_SECURE: "true",
          SESSION_SAMESITE: "Lax",
          SESSION_IDLE_TIMEOUT: "30m",
          SESSION_ABSOLUTE_TIMEOUT: "24h",
          TRUST_PROXY: "true",
          PROXY_HEADER: "X-Forwarded-For",
          ALTCHA_HMAC_KEY: this.env.ALTCHA_HMAC_KEY!,
          REGISTRATION_IP_SALT:
            this.env.REGISTRATION_IP_SALT || this.env.ALTCHA_HMAC_KEY!,
          GAMESERVER_API_KEY: this.env.GAMESERVER_API_KEY || "",
          NETCODE_PRIVATE_KEY: this.env.NETCODE_PRIVATE_KEY || "",
          NETCODE_PROTOCOL_ID: this.env.NETCODE_PROTOCOL_ID || "",
          NETCODE_TOKEN_TTL: this.env.NETCODE_TOKEN_TTL || "60s",
          CLIENT_TOKEN_TTL: this.env.CLIENT_TOKEN_TTL || "24h",
          STUDIO_TOKEN_TTL: this.env.STUDIO_TOKEN_TTL || "168h",
          GSM_PUBLIC_IP: this.env.GSM_PUBLIC_IP || "",
          TYPESAFE_API_KEY: this.env.TYPESAFE_API_KEY || "",
          TYPESAFE_BASE_URL: this.env.TYPESAFE_BASE_URL || "",
          TYPESAFE_MODEL: this.env.TYPESAFE_MODEL || "",
          FILTER_TIMEOUT: this.env.FILTER_TIMEOUT || "20s",
          TRUSTED_PROXIES: this.env.TRUSTED_PROXIES || "0.0.0.0/0,::/0",
          SITELOCK_ENABLED: this.env.SITELOCK_ENABLED || "false",
          SITELOCK_PASSWORD: this.env.SITELOCK_PASSWORD || "",
          LIVETIMER_ENABLED: this.env.LIVETIMER_ENABLED || "false",
          LIVETIMER_DURATION: this.env.LIVETIMER_DURATION || "",
          FILTER_ENABLED: this.env.TYPESAFE_API_KEY ? "true" : "false",
          R2_ACCOUNT_ID: this.env.R2_ACCOUNT_ID,
          R2_BUCKET: this.env.R2_BUCKET || this.env.R2_BUCKET_NAME || "",
          R2_BUCKET_NAME: this.env.R2_BUCKET_NAME || this.env.R2_BUCKET || "",
          R2_ACCESS_KEY_ID: this.env.R2_ACCESS_KEY_ID || this.env.AWS_ACCESS_KEY_ID!,
          R2_SECRET_ACCESS_KEY: this.env.R2_SECRET_ACCESS_KEY || this.env.AWS_SECRET_ACCESS_KEY!,
          R2_PUBLIC_BASE_URL: this.env.R2_PUBLIC_BASE_URL || "",
          UPLOAD_MAX_BYTES: this.env.UPLOAD_MAX_BYTES || String(40 * 1024 * 1024),
          STORAGE_DIR: this.env.STORAGE_DIR || "./uploads",
          R2_ENDPOINT: this.env.R2_ENDPOINT || "",
          AWS_ACCESS_KEY_ID: this.env.R2_ACCESS_KEY_ID || this.env.AWS_ACCESS_KEY_ID!,
          AWS_SECRET_ACCESS_KEY: this.env.R2_SECRET_ACCESS_KEY || this.env.AWS_SECRET_ACCESS_KEY!,
        },
      });
    }

    await container.setInactivityTimeout(INACTIVITY_TIMEOUT_MS);
    const port = container.getTcpPort(APP_PORT);
    let lastError: unknown;

    for (let attempt = 0; attempt < STARTUP_ATTEMPTS; attempt++) {
      try {
        const response = await port.fetch("http://vertexia-container/healthz", {
          signal: AbortSignal.timeout(1500),
        });
        await response.body?.cancel();
        if (response.ok) return;
        lastError = new Error(`Health check returned HTTP ${response.status}`);
      } catch (error) {
        lastError = error;
      }
      await new Promise((resolve) => setTimeout(resolve, STARTUP_RETRY_MS));
    }

    throw new Error("VERTEXIA did not become healthy on port 8080", {
      cause: lastError,
    });
  }
}

export default {
  fetch(request: Request, env: Env): Promise<Response> {
    return env.VERTEXIA_CONTAINER.getByName("vertexia-production").fetch(request);
  },
} satisfies ExportedHandler<Env>;
