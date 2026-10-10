import test from 'node:test';
import assert from 'node:assert/strict';
import {UpdateDiagnostics, browserDescription} from './diagnostics.mjs';

test('retains the latest twenty events and last failure after subsequent state changes', () => {
  let time = 0;
  const log = new UpdateDiagnostics({now: () => `2026-10-09T20:00:${String(time++).padStart(2, '0')}Z`});
  log.start({version: '0.3.15', browser: 'Chrome on Linux'});
  log.record('reconnecting', 'Service discovery disconnected', {failure: true});
  const failure = log.latestFailure;
  for (let index = 0; index < 25; index++) log.record('connecting', `Connection stage ${index}`);
  assert.equal(log.records.length, 20);
  assert.equal(log.records[0].message, 'Connection stage 5');
  assert.equal(log.latestFailure, failure);
  assert.match(log.report(), /Last failure: .*Service discovery disconnected/);
  assert.match(log.report(), /Selected firmware: 0\.3\.15\nBrowser: Chrome on Linux/);
  assert.equal(log.record('connecting', 'Connection stage 24'), false, 'identical consecutive states do not churn the history');
  assert.equal(log.records.length, 20);
});

test('redacts addresses and protected watch/session identifiers from all event fields', () => {
  const log = new UpdateDiagnostics();
  log.start({version: '0.3.15', browser: 'Chrome on Linux'});
  log.protect('private-device-id', 3456789012);
  log.record('error', 'private-device-id C9:9E:15:7A:69:B4 session=3456789012 device id=other-secret',
    {failure: true, stage: 'device identifier=another-secret', code: 'C9:9E:15:7A:69:B4'});
  const report = log.report();
  for (const secret of ['private-device-id', 'C9:9E:15:7A:69:B4', '3456789012', 'other-secret', 'another-secret']) {
    assert.equal(report.includes(secret), false);
  }
  log.record('error', 'a'.repeat(2000));
  assert.equal(log.records.at(-1).message.length, 1000);
  log.start({version: '0.3.16', browser: 'Edge on Windows'});
  assert.equal(log.records.length, 0);
  assert.equal(log.latestFailure, null);
  assert.equal(log.report().includes('0.3.15'), false);
  log.clear();
  assert.equal(log.active, false);
  assert.equal(log.record('connecting', 'No active attempt'), false);
  assert.equal(log.report(), '');
});

test('reports browser version and operating system without retaining a full user agent', () => {
  assert.equal(browserDescription({userAgent: 'Mozilla/5.0 (X11; Linux x86_64) Chrome/130.0.0.0 Safari/537.36'}), 'Chrome 130.0.0.0 on Linux');
  assert.equal(browserDescription({userAgent: 'Windows Chrome/130.0.0.0 Safari/537.36 Edg/130.0.1.2'}), 'Edge 130.0.1.2 on Windows');
  assert.equal(browserDescription({}), 'Browser unavailable on OS unavailable');
});
