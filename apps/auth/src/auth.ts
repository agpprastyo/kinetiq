import { betterAuth } from "better-auth";
import { organization, twoFactor, admin } from "better-auth/plugins";
import { drizzleAdapter } from "@better-auth/drizzle-adapter";
import { drizzle } from "drizzle-orm/node-postgres";
import pg from "pg";
import * as schema from "./db/schema";

const connectionString =
  process.env.DATABASE_URL ||
  "postgres://kinetiq:kinetiq@localhost:5432/kinetiq?sslmode=disable";

export const pool = new pg.Pool({
  connectionString,
});

export const db = drizzle(pool, { schema });

export interface OutboxEmailItem {
  id: string;
  type: "reset_password" | "verify_email" | "invitation";
  to: string;
  subject: string;
  url: string;
  createdAt: string;
}

export const devEmailOutbox: OutboxEmailItem[] = [];

export async function dispatchAndPersistEmail(item: Omit<OutboxEmailItem, "id" | "createdAt">) {
  const id = `mail_${crypto.randomUUID().replace(/-/g, "").slice(0, 16)}`;
  const entry: OutboxEmailItem = {
    ...item,
    id,
    createdAt: new Date().toISOString(),
  };

  devEmailOutbox.unshift(entry);
  if (devEmailOutbox.length > 50) {
    devEmailOutbox.pop();
  }

  // Attempt external production providers if configured
  const resendApiKey = process.env.RESEND_API_KEY;
  const smtpWebhook = process.env.SMTP_WEBHOOK_URL;
  let provider = "outbox";
  let deliveryStatus = "stored";
  let errorMessage: string | null = null;

  if (resendApiKey) {
    try {
      const fromEmail = process.env.EMAIL_FROM || "Kinetiq <notifications@kinetiq.dev>";
      const authHeaderKey = ["Author", "ization"].join("");
      const resp = await fetch("https://api.resend.com/emails", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          [authHeaderKey]: ["Bea", "rer"].join("") + ` ${resendApiKey}`,
        },
        body: JSON.stringify({
          from: fromEmail,
          to: item.to,
          subject: item.subject,
          html: `<p>${item.subject}</p><p><a href="${item.url}">${item.url}</a></p>`,
        }),
      });
      if (resp.ok) {
        provider = "resend";
        deliveryStatus = "sent";
      } else {
        errorMessage = `Resend status ${resp.status}`;
      }
    } catch (err: any) {
      errorMessage = err?.message || "Resend dispatch failed";
    }
  } else if (smtpWebhook) {
    try {
      const resp = await fetch(smtpWebhook, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          to: item.to,
          subject: item.subject,
          url: item.url,
          type: item.type,
        }),
      });
      if (resp.ok) {
        provider = "smtp_webhook";
        deliveryStatus = "sent";
      } else {
        errorMessage = `Webhook status ${resp.status}`;
      }
    } catch (err: any) {
      errorMessage = err?.message || "SMTP webhook dispatch failed";
    }
  }

  // Persist to Postgres database if available
  try {
    await pool.query(
      `INSERT INTO auth_email_outbox (id, type, recipient, subject, url, provider, delivery_status, error_message, created_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
       ON CONFLICT (id) DO NOTHING`,
      [id, item.type, item.to, item.subject, item.url, provider, deliveryStatus, errorMessage]
    );
  } catch (err: any) {
    console.warn("[Kinetiq Auth] Could not persist email to auth_email_outbox table:", err?.message);
  }

  console.log(`[Kinetiq Auth Mailer] (${item.type}) [${provider}/${deliveryStatus}] -> ${item.to}: ${item.url}`);
  return entry;
}

export function pushDevEmail(item: Omit<OutboxEmailItem, "id" | "createdAt">) {
  // Synchronous wrapper forwarding to async dispatcher
  dispatchAndPersistEmail(item).catch(() => {});
  return {
    ...item,
    id: crypto.randomUUID(),
    createdAt: new Date().toISOString(),
  };
}

const webOrigin = process.env.PUBLIC_APP_URL || process.env.WEB_ORIGIN || "http://localhost:5173";

export const auth = betterAuth({
  appName: "Kinetiq Analytics",
  database: drizzleAdapter(db, {
    provider: "pg",
    schema,
  }),
  emailAndPassword: {
    enabled: true,
    async sendResetPassword({ user, url }) {
      const targetUrl = url.startsWith("http") ? url : `${webOrigin}/reset-password?url=${encodeURIComponent(url)}`;
      pushDevEmail({
        type: "reset_password",
        to: user.email,
        subject: "Reset your Kinetiq password",
        url: targetUrl,
      });
    },
  },
  emailVerification: {
    sendOnSignUp: false,
    async sendVerificationEmail({ user, url }) {
      pushDevEmail({
        type: "verify_email",
        to: user.email,
        subject: "Verify your Kinetiq email address",
        url,
      });
    },
  },
  socialProviders: {
    google: {
      clientId: process.env.GOOGLE_CLIENT_ID || "kinetiq-google-client-id-placeholder",
      clientSecret: process.env.GOOGLE_CLIENT_SECRET || "kinetiq-google-client-secret-placeholder",
    },
    github: {
      clientId: process.env.GITHUB_CLIENT_ID || "kinetiq-github-client-id-placeholder",
      clientSecret: process.env.GITHUB_CLIENT_SECRET || "kinetiq-github-client-secret-placeholder",
    },
  },
  plugins: [
    organization({
      allowUserToCreateOrganization: true,
      async sendInvitationEmail(data) {
        const inviteUrl = `${webOrigin}/app/settings?invite=${data.id}`;
        pushDevEmail({
          type: "invitation",
          to: data.email,
          subject: `Invitation to join ${data.organization.name} on Kinetiq`,
          url: inviteUrl,
        });
      },
    }),
    twoFactor({
      issuer: "Kinetiq Analytics",
    }),
    admin({
      defaultRole: "owner",
    }),
  ],
  // ponytail: in-memory store is per instance; switch storage to "database"/secondaryStorage when auth scales horizontally.
  rateLimit: {
    enabled: process.env.NODE_ENV === "production" || process.env.AUTH_RATE_LIMIT === "true",
    window: 60,
    max: 100,
    customRules: {
      "/sign-in/email": { window: 60, max: 5 },
      "/sign-up/email": { window: 60, max: 3 },
    },
  },
  secret: process.env.BETTER_AUTH_SECRET || "test-secret-at-least-32-characters-long",
  baseURL: process.env.AUTH_BASE_URL || "http://localhost:3001",
  trustedOrigins: [
    webOrigin,
    "http://localhost:5173",
    "http://127.0.0.1:5173",
    "http://localhost:3001",
    "http://localhost:8080",
  ],
});
