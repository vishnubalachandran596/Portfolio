'use strict';
// Shared-secret check for api/run.js. The Go app signs each call with an
// HMAC of the request body keyed by JWT_SECRET (both functions already have
// it), so nothing new has to be configured and a browser can't call the
// runner: it has no way to sign.
const crypto = require('node:crypto');

const MAX_SKEW_MS = 60 * 1000;

function sign(secret, ts, body) {
  const digest = crypto.createHash('sha256').update(body).digest('hex');
  return crypto.createHmac('sha256', secret).update(`runner:${ts}:${digest}`).digest('hex');
}

// header looks like "<unix-ms>.<hex-hmac>".
function verify(secret, header, body, now = Date.now()) {
  if (!secret || typeof header !== 'string') return false;
  const [ts, mac] = header.split('.');
  if (!/^\d{10,16}$/.test(ts || '') || !/^[0-9a-f]{64}$/.test(mac || '')) return false;
  if (Math.abs(now - Number(ts)) > MAX_SKEW_MS) return false;
  const want = Buffer.from(sign(secret, ts, body), 'hex');
  const got = Buffer.from(mac, 'hex');
  return want.length === got.length && crypto.timingSafeEqual(want, got);
}

module.exports = { sign, verify };
