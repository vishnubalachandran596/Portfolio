'use strict';
// POST /api/run — the server-side code runner behind the course reader's
// editor. The Go app calls it (never a browser): it checks the buyer's
// access, then sends {lang, code} here with an HMAC header (see
// _runner/verify.js). This function loads the JDoodle embed for that language
// in a headless browser, puts the code in, runs it and returns the output.
// Embed keys come from JDOODLE_EMBED_KEYS and live only in this function's
// environment.
const { verify } = require('./_runner/verify.js');
const { run } = require('./_runner/embed.js');

const MAX_BODY = 64 * 1024;
const MAX_CODE = 20000;
const MAX_PARALLEL = 2;

let browserPromise = null;
let active = 0;

function parseKeys(raw) {
  const out = {};
  for (const pair of (raw || '').split(',')) {
    const i = pair.indexOf('=');
    if (i > 0) out[pair.slice(0, i).trim().toLowerCase()] = pair.slice(i + 1).trim();
  }
  return out;
}

async function launch() {
  // puppeteer-core and @sparticuz/chromium are ES modules; this file is
  // CommonJS, and require() of an ES module fails on Vercel's Node 22.
  const puppeteer = (await import('puppeteer-core')).default;
  if (process.env.CHROMIUM_PATH) {
    return puppeteer.launch({ executablePath: process.env.CHROMIUM_PATH, headless: 'shell', args: ['--no-sandbox'] });
  }
  const chromium = (await import('@sparticuz/chromium')).default;
  return puppeteer.launch({
    executablePath: await chromium.executablePath(),
    args: chromium.args,
    headless: 'shell',
  });
}

async function getBrowser() {
  if (browserPromise) {
    const b = await browserPromise.catch(() => null);
    if (b && b.connected) return b;
  }
  browserPromise = launch();
  return browserPromise;
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let n = 0;
    req.on('data', (c) => {
      n += c.length;
      if (n > MAX_BODY) { reject(Object.assign(new Error('too large'), { status: 413 })); req.destroy(); return; }
      chunks.push(c);
    });
    req.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    req.on('error', reject);
  });
}

function send(res, status, obj) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.setHeader('Cache-Control', 'no-store');
  res.end(JSON.stringify(obj));
}

module.exports = async (req, res) => {
  if (req.method !== 'POST') return send(res, 405, { error: 'method not allowed' });
  let body;
  try { body = await readBody(req); } catch (e) { return send(res, e.status || 400, { error: 'bad request' }); }
  // JWT_SECRET is the same value the Go app signs with.
  if (!verify(process.env.JWT_SECRET, req.headers['x-runner-token'], body)) return send(res, 401, { error: 'unauthorized' });

  let msg;
  try { msg = JSON.parse(body); } catch (e) { return send(res, 400, { error: 'bad json' }); }
  const lang = String(msg.lang || '').toLowerCase();
  const code = typeof msg.code === 'string' ? msg.code : '';
  const key = parseKeys(process.env.JDOODLE_EMBED_KEYS)[lang];
  if (!key) return send(res, 404, { error: 'no compiler for that language' });
  if (!code.trim() || code.length > MAX_CODE) return send(res, 400, { error: 'code is empty or too long' });
  if (active >= MAX_PARALLEL) return send(res, 429, { error: 'busy' });

  active++;
  try {
    const origin = (process.env.SITE_URL || 'https://www.vishnubalachandran.com').replace(/\/+$/, '');
    const browser = await getBrowser();
    const r = await run(browser, { key, code, origin, debug: msg.debug === true });
    if (r.blocked) return send(res, 503, { error: 'captcha' });
    const out = { ok: r.ok, timedOut: r.timedOut, output: r.output.slice(0, 20000), ms: r.ms };
    if (msg.debug === true) { out.trace = r.trace.filter((m) => typeof m !== 'string' || !m.includes('heightxPYMx')); out.domText = r.domText; out.shot = r.shot; }
    return send(res, 200, out);
  } catch (e) {
    browserPromise = null; // a crashed browser is relaunched next time
    console.error('run failed:', e && e.message);
    return send(res, 502, { error: 'the compiler could not run that' });
  } finally {
    active--;
  }
};
