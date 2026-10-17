'use strict';
// Drives one JDoodle embed in a headless browser and returns what it printed.
//
// The embed is JDoodle's own widget: a page that loads inside a host page
// through jdoodle-pym.min.js, which gives the host sendMessage('setCode' |
// 'execute') and posts the embed's replies back. A real browser loads it
// here, exactly as a visitor's browser would, so the embed key never reaches
// a visitor and the run counts against the owner's plan like any other run.
const JDOODLE = process.env.JDOODLE_BASE || 'https://www.jdoodle.com';

const RUN_TIMEOUT_MS = 30000;
const LOAD_TIMEOUT_MS = 20000;

function hostPage(key, code) {
  const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;');
  return `<!doctype html><html><head><meta charset="utf-8"></head><body>
<div id="e" data-pym-src="${JDOODLE}/embed/v1/${encodeURIComponent(key)}">${esc(code)}</div>
<script>
window.__msgs = [];
window.addEventListener('message', function (e) {
  if (e.origin !== ${JSON.stringify(JDOODLE)}) return;
  var d = e.data;
  try { window.__msgs.push(typeof d === 'string' ? d : JSON.parse(JSON.stringify(d))); } catch (err) {}
  if (window.__onmsg) window.__onmsg(d);
});
</script>
<script src="${JDOODLE}/assets/jdoodle-pym.min.js"></script>
</body></html>`;
}

// Pull printable text out of whatever shape a message has. The embed
// replies with objects keyed by what they carry ({onLoad}, {script},
// {output}, {result}); only output and result carry program text.
function outputOf(d) {
  if (!d || typeof d !== 'object') return null;
  for (const k of ['output', 'result']) {
    const v = d[k];
    if (typeof v === 'string') return { kind: k, text: v };
    if (v && typeof v === 'object') {
      const t = v.output ?? v.stdout ?? v.text ?? v.result;
      if (typeof t === 'string') return { kind: k, text: t, meta: v };
    }
  }
  return null;
}

async function run(browser, { key, code, origin }) {
  const page = await browser.newPage(); if (process.env.RUNNER_DEBUG) { page.on('request', r => console.error('req', r.url())); page.on('requestfailed', r => console.error('fail', r.url(), r.failure() && r.failure().errorText)); page.on('console', m => console.error('console', m.text())); }
  try {
    await page.setRequestInterception(true);
    const hostURL = `${origin}/__runner`;
    page.on('request', (req) => {
      const u = req.url();
      if (u === hostURL) {
        req.respond({ status: 200, contentType: 'text/html; charset=utf-8', body: hostPage(key, code) });
      } else if (/googletagmanager|google-analytics|doubleclick|facebook\.net|clarity\.ms/.test(u)) {
        req.abort();
      } else {
        req.continue();
      }
    });
    await page.goto(hostURL, { waitUntil: 'domcontentloaded', timeout: LOAD_TIMEOUT_MS });

    // Wait for the embed to say it is listening.
    await page.waitForFunction(
      () => window.__msgs.some((m) => m && typeof m === 'object' && m.onLoad),
      { timeout: LOAD_TIMEOUT_MS },
    );

    // Hand it the code, run it, and wait for the program's result.
    const started = Date.now();
    const finished = page.evaluate((limit) => new Promise((resolve) => {
      const done = () => resolve(true);
      const timer = setTimeout(() => resolve(false), limit);
      window.__onmsg = (d) => {
        if (d && typeof d === 'object' && d.result !== undefined) { clearTimeout(timer); setTimeout(done, 150); }
      };
    }), RUN_TIMEOUT_MS);
    await page.evaluate((c) => {
      const inst = pym.autoInitInstances[pym.autoInitInstances.length - 1];
      inst.sendMessage('setCode', c);
      inst.sendMessage('execute');
    }, code);
    const completed = await finished;

    const msgs = await page.evaluate(() => window.__msgs);
    let text = '';
    let lastResult = null;
    for (const m of msgs) {
      const o = outputOf(m);
      if (!o) continue;
      if (o.kind === 'output') text += o.text;
      else lastResult = o;
    }
    if (!text && lastResult) text = lastResult.text;
    return { ok: completed, timedOut: !completed, output: text, ms: Date.now() - started, trace: msgs };
  } finally {
    await page.close().catch(() => {});
  }
}

module.exports = { run, outputOf, hostPage };
