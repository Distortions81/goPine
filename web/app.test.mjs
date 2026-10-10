import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {runInNewContext} from 'node:vm';
import {webcrypto} from 'node:crypto';
import {browserSupport, connectionReadiness} from './support.mjs';
import {MAX_PACKAGE_BYTES} from './firmware.mjs';
import {UpdateError} from './protocol.mjs';

const source = readFileSync(new URL('./app.mjs', import.meta.url), 'utf8');
const html = readFileSync(new URL('./index.html', import.meta.url), 'utf8');
const manifest = {schema: 1, firmware: {
  version: '0.3.15', package: 'firmware/gopine-dfu-0.3.15.zip', sha256: 'a'.repeat(64),
  releaseUrl: 'https://github.com/Distortions81/goPineTime/releases/tag/v0.3.15',
}};
const packageBytes = Uint8Array.of(1, 2, 3, 4);
const checked = {version: '0.3.15', image: Uint8Array.of(5, 6, 7), sha256: manifest.firmware.sha256};
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return {promise, resolve, reject};
}
const tick = () => new Promise(resolve => setImmediate(resolve));
async function until(condition) {
  for (let i = 0; i < 40; i++) { if (condition()) return; await tick(); }
  assert.ok(condition(), 'app did not reach the expected state');
}

class Element {
  constructor(tag) {
    this.textContent = ''; this.value = ''; this.files = [];
    this.hidden = /\bhidden(?:\s|>)/.test(tag);
    this.disabled = /\bdisabled(?:\s|>)/.test(tag);
    this.style = {}; this.listeners = new Map(); this.attributes = new Map();
    const classes = new Set();
    this.classList = {add: value => classes.add(value), toggle: (value, enabled) => {
      if (enabled) classes.add(value); else classes.delete(value);
    }};
  }
  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(listener);
  }
  async emit(type, event = {}) {
    await Promise.all((this.listeners.get(type) || []).map(listener => listener({target: this, ...event})));
  }
  setAttribute(name, value) { this.attributes.set(name, value); }
  removeAttribute(name) { this.attributes.delete(name); }
  focus() {}
  select() {}
}

// Execute the complete application with isolated DOM/network/protocol boundaries.
// Package parsing and the Bluetooth protocol have their own non-mocked test suites.
function app(options = {}) {
  const elements = new Map([...html.matchAll(/<[^>]+\bid="([^"]+)"[^>]*>/g)]
    .map(match => [match[1], new Element(match[0])]));
  const get = id => { assert.ok(elements.has(id), `missing HTML element ${id}`); return elements.get(id); };
  const calls = {fetch: [], inspect: [], picker: 0, updaters: []};
  const document = Object.assign(new Element(''), {getElementById: get, visibilityState: 'visible'});
  const window = Object.assign(new Element(''), {isSecureContext: true});
  const navigator = {bluetooth: {requestDevice: async () => {
    calls.picker++;
    if (!options.updaterRun) throw new DOMException('Picker cancelled', 'NotFoundError');
    return {};
  }}};
  class DirectUpdater {
    constructor(config) {
      Object.assign(this, config, {pendingOperations: 0, active: false, runs: 0, disconnects: 0});
      calls.updaters.push(this);
    }
    async run() {
      this.runs++; this.active = true;
      try { return await options.updaterRun(this); }
      finally { this.active = false; }
    }
    disconnect() { this.disconnects++; }
    stop() {}
  }
  assert.equal((source.match(/^import .*;$/gm) || []).length, 3, 'update dependency injection when app imports change');
  runInNewContext(source.replace(/^import .*;\n/gm, ''), {
    document, window, navigator, location: {href: 'https://distortions81.github.io/goPineTime/'},
    URL, Uint8Array, DOMException, DecompressionStream: options.validation === false ? undefined : DecompressionStream,
    AbortController, setTimeout: options.setTimeout || setTimeout, clearTimeout: options.clearTimeout || clearTimeout,
    crypto: webcrypto, MAX_PACKAGE_BYTES, browserSupport, connectionReadiness, DirectUpdater, UpdateError,
    requestWatch: () => navigator.bluetooth.requestDevice(),
    fetch: async (...args) => {
      calls.fetch.push(args);
      return options.fetch ? options.fetch(...args) : new Response(JSON.stringify(manifest));
    },
    inspectPackage: async (...args) => {
      calls.inspect.push(args);
      return options.inspect ? options.inspect(...args) : checked;
    },
  }, {filename: 'web/app.mjs'});
  return {get, calls, async file(name = 'my-build.zip', bytes = packageBytes) {
    get('file').files = [{name, size: bytes.byteLength, arrayBuffer: async () => bytes.buffer}];
    await get('file').emit('change');
  }};
}

test('latest release downloads by default, stays disabled through validation, and never opens Bluetooth automatically', async () => {
  const download = deferred(), validation = deferred();
  const page = app({fetch: url => url.endsWith('.json') ? new Response(JSON.stringify(manifest)) : download.promise,
    inspect: () => validation.promise});
  await until(() => page.calls.fetch.length === 2);
  assert.equal(page.calls.fetch[1][0], manifest.firmware.package);
  assert.equal(page.get('connect').disabled, true);
  assert.equal(page.get('file').disabled, true);
  download.resolve(new Response(new ReadableStream({start(controller) {
    controller.enqueue(packageBytes.subarray(0, 2)); controller.enqueue(packageBytes.subarray(2)); controller.close();
  }})));
  await until(() => page.calls.inspect.length === 1);
  assert.deepEqual(new Uint8Array(page.calls.inspect[0][0]), packageBytes);
  assert.equal(page.calls.inspect[0][1].expectedSha256, manifest.firmware.sha256);
  assert.equal(page.get('connect').disabled, true);
  validation.resolve(checked);
  await until(() => !page.get('connect').disabled);
  assert.equal(page.get('validated').hidden, false);
  assert.match(page.get('file-name').textContent, /0\.3\.15.*published release/);
  assert.equal(page.calls.picker, 0);
  await page.get('connect').emit('click');
  assert.equal(page.calls.picker, 1, 'connection requires an explicit click');
});

test('a manifest arriving while the native file picker is open preserves the later file choice', async () => {
  const release = deferred();
  const page = app({fetch: () => release.promise});
  await page.get('file').emit('click');
  release.resolve(new Response(JSON.stringify(manifest)));
  await until(() => !page.get('latest').disabled);
  assert.equal(page.calls.fetch.length, 1);
  assert.equal(page.get('file').disabled, false);
  await page.file('chosen-in-picker.zip');
  assert.equal(page.get('file-name').textContent, 'chosen-in-picker.zip');
  assert.equal(page.get('connect').disabled, false);
  assert.equal(page.calls.fetch.length, 1);
});

test('cancelling the native file picker leaves the published release button usable', async () => {
  const release = deferred();
  const page = app({fetch: url => url.endsWith('.json') ? release.promise : new Response(packageBytes)});
  await page.get('file').emit('click');
  release.resolve(new Response(JSON.stringify(manifest)));
  await until(() => !page.get('latest').disabled);
  await page.get('file').emit('cancel');
  assert.equal(page.calls.fetch.length, 1);
  assert.equal(page.get('connect').disabled, true);
  await page.get('latest').emit('click');
  assert.equal(page.calls.fetch.length, 2);
  assert.equal(page.get('validated').hidden, false);
  assert.equal(page.get('connect').disabled, false);
});

for (const valid of [true, false]) {
  for (const duringValidation of [true, false]) {
    test(`delayed manifest preserves a ${valid ? 'valid' : 'rejected'} manual choice ${duringValidation ? 'during' : 'after'} validation`, async () => {
      const release = deferred(), validation = deferred();
      const page = app({fetch: () => release.promise, inspect: () => validation.promise});
      const selecting = page.file('local-build.zip');
      await until(() => page.calls.inspect.length === 1);
      if (duringValidation) { release.resolve(new Response(JSON.stringify(manifest))); await tick(); }
      if (valid) validation.resolve({...checked, version: '0.3.16'});
      else validation.reject(new Error('Invalid local firmware'));
      await selecting;
      if (!duringValidation) release.resolve(new Response(JSON.stringify(manifest)));
      await until(() => !page.get('latest').disabled);
      await tick();
      assert.equal(page.calls.fetch.length, 1, 'the release must not replace a deliberate file choice');
      assert.equal(page.calls.inspect.length, 1);
      assert.equal(page.get('connect').disabled, !valid);
      if (valid) assert.equal(page.get('file-name').textContent, 'local-build.zip');
      else assert.match(page.get('error').textContent, /Invalid local firmware/);
    });
  }
}

test('failed automatic download leaves manual ZIP selection usable', async () => {
  const page = app({fetch: url => url.endsWith('.json') ? new Response(JSON.stringify(manifest)) : new Response('', {status: 503})});
  await until(() => /503/.test(page.get('error').textContent));
  assert.equal(page.get('file').disabled, false);
  assert.equal(page.get('latest').disabled, false);
  assert.equal(page.get('connect').disabled, true);
  assert.equal(page.calls.inspect.length, 0);
  await page.file();
  assert.equal(page.get('connect').disabled, false);
  assert.equal(page.get('error').hidden, true);
  assert.equal(page.calls.picker, 0);
});

test('failed automatic validation can retry through the release button', async () => {
  let attempts = 0;
  const page = app({fetch: url => new Response(url.endsWith('.json') ? JSON.stringify(manifest) : packageBytes),
    inspect: async () => { if (++attempts === 1) throw new Error('Package SHA-256 mismatch'); return checked; }});
  await until(() => /SHA-256/.test(page.get('error').textContent));
  assert.equal(page.get('connect').disabled, true);
  await page.get('latest').emit('click');
  assert.equal(page.calls.inspect.length, 2);
  assert.equal(page.get('connect').disabled, false);
  assert.equal(page.get('error').hidden, true);
  assert.equal(page.calls.picker, 0);
});

for (const [name, response] of [
  ['missing', () => new Response(JSON.stringify({schema: 1, firmware: null}))],
  ['unavailable', () => new Response('', {status: 503})],
  ['invalid', () => new Response(JSON.stringify({...manifest, schema: 99}))],
]) {
  test(`${name} release metadata replaces the loading status and allows a manual ZIP`, async () => {
    const page = app({fetch: response});
    await until(() => /Choose a goPine OTA ZIP/.test(page.get('status').textContent));
    assert.equal(page.get('file').disabled, false);
    assert.equal(page.get('latest').disabled, true);
    assert.equal(page.get('connect').disabled, true);
    assert.equal(page.calls.fetch.length, 1);
    await page.file();
    assert.equal(page.get('connect').disabled, false);
  });
}

for (const stalledAt of ['headers', 'body']) {
  test(`automatic download timeout at ${stalledAt} unlocks a manual ZIP fallback`, async () => {
    const timers = new Map();
    let nextTimer = 0, aborted = false;
    const page = app({
      setTimeout: callback => { const id = ++nextTimer; timers.set(id, callback); return id; },
      clearTimeout: id => timers.delete(id),
      fetch: (url, {signal} = {}) => {
        if (url.endsWith('.json')) return new Response(JSON.stringify(manifest));
        if (stalledAt === 'headers') return new Promise((resolve, reject) => {
          signal.addEventListener('abort', () => { aborted = true; reject(new DOMException('Aborted', 'AbortError')); });
        });
        return new Response(new ReadableStream({start(controller) {
          controller.enqueue(packageBytes.subarray(0, 2));
          signal.addEventListener('abort', () => { aborted = true; controller.error(new DOMException('Aborted', 'AbortError')); });
        }}));
      },
    });
    await until(() => page.calls.fetch.length === 2);
    assert.equal(page.get('file').disabled, true);
    assert.equal(timers.size, 1);
    [...timers.values()][0]();
    await until(() => /download timed out/.test(page.get('error').textContent));
    assert.equal(aborted, true);
    assert.equal(timers.size, 0);
    assert.equal(page.calls.inspect.length, 0, 'partial bytes must not reach validation');
    assert.equal(page.get('file').disabled, false);
    assert.equal(page.get('latest').disabled, false);
    assert.equal(page.get('connect').disabled, true);
    await page.file();
    assert.equal(page.get('connect').disabled, false);
    assert.equal(page.get('error').hidden, true);
  });
}

test('unsupported validation does not download firmware automatically', async () => {
  const page = app({validation: false});
  await until(() => /0\.3\.15/.test(page.get('release-description').textContent));
  await tick();
  assert.equal(page.calls.fetch.length, 1);
  assert.equal(page.calls.inspect.length, 0);
  assert.equal(page.get('latest').disabled, true);
  assert.equal(page.get('connect').disabled, true);
});

test('a retired native Bluetooth operation prevents Resume and Start over until it settles', async () => {
  const page = app({fetch: url => new Response(url.endsWith('.json') ? JSON.stringify(manifest) : packageBytes),
    updaterRun: async updater => {
      updater.pendingOperations = 1;
      throw new UpdateError('Bluetooth request is still closing', {retryable: true});
    }});
  await until(() => !page.get('connect').disabled);
  await page.get('connect').emit('click');
  const updater = page.calls.updaters[0];
  assert.equal(page.get('resume').hidden, false);
  assert.equal(page.get('reset').hidden, false);
  assert.equal(page.get('resume').disabled, true);
  assert.equal(page.get('reset').disabled, true);
  // Synthetic events also verify the handlers guard the retired native request.
  await page.get('resume').emit('click');
  await page.get('reset').emit('click');
  assert.equal(updater.runs, 1);
  assert.equal(updater.disconnects, 0);
  assert.equal(page.get('connect').hidden, true);
  updater.pendingOperations = 0;
  updater.onPendingChange(0);
  assert.equal(page.get('resume').disabled, false);
  assert.equal(page.get('reset').disabled, false);
  await page.get('reset').emit('click');
  assert.equal(updater.disconnects, 1);
  assert.equal(page.get('connect').hidden, false);
  assert.equal(page.get('connect').disabled, false);
});
