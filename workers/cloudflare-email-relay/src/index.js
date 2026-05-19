import PostalMime from "postal-mime";
import { convert } from "html-to-text";

export default {
  async fetch() {
    return new Response("cloudflare-email-relay worker is running", { status: 200 });
  },

  async email(message, env) {
    const event = await buildEmailEvent(message);
    const ok = await forwardEmailEvent(env, event);
    if (!ok && env.WEBHOOK_FAIL_OPEN !== "true") {
      message.setReject("mailbox webhook delivery failed");
    }
  },
};

async function buildEmailEvent(message) {
  const parsed = await PostalMime.parse(message.raw);
  const textBody = clean(
    parsed.text ||
      convert(parsed.html || "", {
        wordwrap: false,
        selectors: [
          { selector: "img", format: "skip" },
          { selector: "style", format: "skip" },
          { selector: "script", format: "skip" },
          { selector: "head", format: "skip" },
          { selector: "a", options: { ignoreHref: true } },
        ],
      }),
  );
  const recipients = uniqueEmails([
    message.to,
    ...(parsed.to || []).map((item) => item.address),
    ...(parsed.cc || []).map((item) => item.address),
    ...(parsed.bcc || []).map((item) => item.address),
  ]);
  const messageId = cleanHeader(parsed.messageId || message.headers.get("message-id") || "");
  const receivedAtUnix = receivedAt(parsed.date || message.headers.get("date"));
  return {
    version: 1,
    provider: "cloudflare",
    eventId: await stableEventId("cloudflare", messageId, message.from, recipients.join(","), receivedAtUnix),
    messageId,
    fromAddress: normalizeEmail(parsed.from?.address || message.from || ""),
    recipients,
    subject: cleanHeader(parsed.subject || message.headers.get("subject") || ""),
    textBody,
    htmlBody: String(parsed.html || ""),
    receivedAtUnix,
    rawSize: Number(message.rawSize || 0),
  };
}

async function forwardEmailEvent(env, event) {
  const url = String(env.MAILBOX_WEBHOOK_URL || "").trim();
  const token = String(env.MAILBOX_WEBHOOK_TOKEN || "").trim();
  if (!url || !token) {
    console.error("MAILBOX_WEBHOOK_URL and MAILBOX_WEBHOOK_TOKEN are required");
    return false;
  }
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Webhook-Token": token,
    },
    body: JSON.stringify(event),
  });
  if (!response.ok) {
    console.error(`mailbox webhook failed status=${response.status} body=${await response.text()}`);
    return false;
  }
  return true;
}

function clean(value) {
  return String(value || "")
    .replace(/\r\n?/g, "\n")
    .replace(/[\u200B-\u200D\uFEFF]/g, "")
    .replace(/\u00A0/g, " ")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function cleanHeader(value) {
  return clean(value).replace(/\n/g, " ");
}

function uniqueEmails(values) {
  const out = [];
  const seen = new Set();
  for (const value of values) {
    const email = normalizeEmail(value);
    if (!email || seen.has(email)) continue;
    seen.add(email);
    out.push(email);
  }
  return out;
}

function normalizeEmail(value) {
  return String(value || "").trim().toLowerCase();
}

function receivedAt(value) {
  const parsed = Date.parse(value || "");
  if (Number.isFinite(parsed)) return Math.floor(parsed / 1000);
  return Math.floor(Date.now() / 1000);
}

async function stableEventId(...parts) {
  const bytes = new TextEncoder().encode(parts.join("\0"));
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}
