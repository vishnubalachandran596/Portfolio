'use strict';
// node --test api/_runner/   (needs CHROMIUM_PATH pointing at a Chromium)
// A stand-in for jdoodle.com: the same pym host/embed message protocol, with
// a fake "program" that upper-cases its input.
const test = require('node:test');
const assert = require('node:assert');
const http = require('node:http');
const { sign, verify } = require('./verify.js');

test('verify accepts only a fresh, correctly signed body', () => {
  const secret = 'unit-test-secret-unit-test-secret';
  const body = JSON.stringify({ lang: 'python', code: 'print(1)' });
  const ts = String(Date.now());
  const header = `${ts}.${sign(secret, ts, body)}`;
  assert.ok(verify(secret, header, body));
  assert.ok(!verify(secret, header, body + ' '), 'body change');
  assert.ok(!verify('other-secret', header, body), 'wrong key');
  assert.ok(!verify('', header, body), 'unset secret');
  assert.ok(!verify(secret, undefined, body), 'no header');
  assert.ok(!verify(secret, `${ts}.zz`, body), 'bad mac');
  const old = String(Date.now() - 5 * 60 * 1000);
  assert.ok(!verify(secret, `${old}.${sign(secret, old, body)}`, body), 'stale');
});

test('Node verifier accepts the signature the Go client produces', () => {
  // Same vector as core/coderun/client_test.go.
  const secret = 'unit-test-secret-unit-test-secret';
  const mac = 'dfb959cea7c18b1e21b4cfa4c03e527bb1d7d9586711c646fb4027ead6895221';
  assert.strictEqual(sign(secret, '1760000000000', '{"a":1}'), mac);
  assert.ok(verify(secret, `1760000000000.${mac}`, '{"a":1}', 1760000000000 + 1000));
});

const PYM = `
(function () {
  var holder = document.querySelector('[data-pym-src]');
  var f = document.createElement('iframe');
  f.src = holder.getAttribute('data-pym-src');
  f.dataset.code = holder.textContent;
  holder.appendChild(f);
  window.pym = { autoInitInstances: [{ sendMessage: function (t, d) {
    var m = {}; m[t] = d === undefined ? true : d; f.contentWindow.postMessage(m, '*');
  } }] };
})();`;
const EMBED = `<!doctype html><script>
var code = '';
window.addEventListener('message', function (e) {
  if (e.data.setCode !== undefined) code = e.data.setCode;
  if (e.data.execute) setTimeout(function () {
    parent.postMessage({ output: 'OUT:' + code.toUpperCase() }, '*');
    parent.postMessage({ result: { statusCode: 200, cpuTime: '0.01' } }, '*');
  }, 50);
});
parent.postMessage({ onLoad: true }, '*');
</script>`;

function mockJDoodle() {
  return new Promise((resolve) => {
    const srv = http.createServer((req, res) => {
      if (req.url.startsWith('/assets/jdoodle-pym.min.js')) { res.setHeader('Content-Type', 'text/javascript'); res.end(PYM); }
      else if (req.url.startsWith('/embed/v1/')) { res.setHeader('Content-Type', 'text/html'); res.end(EMBED); }
      else { res.statusCode = 404; res.end(); }
    }).listen(0, '127.0.0.1', () => resolve(srv));
  });
}

test('run() loads the embed, sends the code and returns the output', { skip: !process.env.CHROMIUM_PATH }, async () => {
  const srv = await mockJDoodle();
  process.env.JDOODLE_BASE = `http://127.0.0.1:${srv.address().port}`;
  delete require.cache[require.resolve('./embed.js')];
  const { run } = require('./embed.js');
  const puppeteer = (await import('puppeteer-core')).default;
  const browser = await puppeteer.launch({ executablePath: process.env.CHROMIUM_PATH, headless: 'shell', args: ['--no-sandbox', '--no-proxy-server'] });
  try {
    const r = await run(browser, { key: 'k', code: 'print("hi")', origin: process.env.JDOODLE_BASE });
    assert.strictEqual(r.ok, true);
    assert.strictEqual(r.output, 'OUT:PRINT("HI")');
  } finally {
    await browser.close();
    srv.close();
  }
});
