// Copyright (C) 2026 ItsWempo
// SPDX-License-Identifier: AGPL-3.0-or-later

import { DurableObject } from "cloudflare:workers";

const APP_PORT = 3000;
const INACTIVITY_TIMEOUT_MS = 30 * 60 * 1000;
const STARTUP_ATTEMPTS = 90;
const STARTUP_RETRY_MS = 500;

type VertexiaRuntimeEnv = Env & {
  DB_HOST?: string;
  DB_PORT?: string;
  DB_USER?: string;
  DB_PASS?: string;
  DB_NAME?: string;
  ALTCHA_HMAC_KEY?: string;
  REGISTRATION_IP_SALT?: string;
  AWS_ACCESS_KEY_ID?: string;
  AWS_SECRET_ACCESS_KEY?: string;
  R2_ACCOUNT_ID?: string;
  R2_BUCKET_NAME?: string;
  TYPESAFE_API_KEY?: string;
  GAMESERVER_API_KEY?: string;
  GSM_AGENT_URL?: string;
  GSM_AGENT_TOKEN?: string;
  GSM_PUBLIC_HOST?: string;
  GSM_PORT_MIN?: string;
  GSM_PORT_MAX?: string;
  REDIS_ADDR?: string;
  REDIS_PASSWORD?: string;
  REDIS_DB?: string;
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
      const forwarded = new Request(url, request);
      forwarded.headers.delete("host");

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
      ["AWS_ACCESS_KEY_ID", this.env.AWS_ACCESS_KEY_ID],
      ["AWS_SECRET_ACCESS_KEY", this.env.AWS_SECRET_ACCESS_KEY],
    ];
    const missing = required.filter(([, value]) => !value).map(([name]) => name);
    if (missing.length > 0) {
      throw new Error(`Missing required Worker secrets: ${missing.join(", ")}`);
    }
    if (!this.env.R2_ACCOUNT_ID || !this.env.R2_BUCKET_NAME) {
      throw new Error("R2_ACCOUNT_ID and R2_BUCKET_NAME must be configured");
    }

    const gsmConfigured = Boolean(
      this.env.GSM_AGENT_URL &&
      this.env.GSM_AGENT_TOKEN &&
      this.env.GSM_PUBLIC_HOST &&
      this.env.REDIS_ADDR,
    );

    if (!container.running) {
      container.start({
        image: container.images.base,
        instance: "standard-1",
        enableInternet: true,
        env: {
          PORT: String(APP_PORT),
          ADDR: `:${APP_PORT}`,
          RUN_WITH_DATABASE: "true",
          DB_HOST: this.env.DB_HOST!,
          DB_PORT: this.env.DB_PORT || "3306",
          DB_USER: this.env.DB_USER!,
          DB_PASS: this.env.DB_PASS!,
          DB_NAME: this.env.DB_NAME!,
          DB_TLS: "true",
          DB_MAX_OPEN_CONNS: "20",
          DB_MAX_IDLE_CONNS: "5",
          DB_CONN_MAX_LIFETIME: "5m",
          GSM_ENABLED: gsmConfigured ? "true" : "false",
          GSM_AGENT_URL: this.env.GSM_AGENT_URL || "",
          GSM_AGENT_TOKEN: this.env.GSM_AGENT_TOKEN || "",
          GSM_PUBLIC_HOST: this.env.GSM_PUBLIC_HOST || "",
          GSM_PORT_MIN: this.env.GSM_PORT_MIN || "5100",
          GSM_PORT_MAX: this.env.GSM_PORT_MAX || "5250",
          REDIS_ADDR: this.env.REDIS_ADDR || "",
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
          TYPESAFE_API_KEY: this.env.TYPESAFE_API_KEY || "",
          FILTER_ENABLED: this.env.TYPESAFE_API_KEY ? "true" : "false",
          R2_ACCOUNT_ID: this.env.R2_ACCOUNT_ID,
          R2_BUCKET_NAME: this.env.R2_BUCKET_NAME,
          AWS_ACCESS_KEY_ID: this.env.AWS_ACCESS_KEY_ID!,
          AWS_SECRET_ACCESS_KEY: this.env.AWS_SECRET_ACCESS_KEY!,
        },
      });
    }

    await container.setInactivityTimeout(INACTIVITY_TIMEOUT_MS);
    const port = container.getTcpPort(APP_PORT);
    let lastError: unknown;

    for (let attempt = 0; attempt < STARTUP_ATTEMPTS; attempt++) {
      try {
        const response = await port.fetch("http://vertexia-container/health", {
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

    throw new Error("VERTEXIA did not become healthy on port 3000", {
      cause: lastError,
    });
  }
}

export default {
  fetch(request: Request, env: Env): Promise<Response> {
    return env.VERTEXIA_CONTAINER.getByName("vertexia-production").fetch(request);
  },
} satisfies ExportedHandler<Env>;
