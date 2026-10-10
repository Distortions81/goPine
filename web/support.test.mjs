import test from 'node:test';
import assert from 'node:assert/strict';
import { browserSupport, connectionReadiness } from './support.mjs';

const supported = browserSupport({ secure: true, bluetooth: true, validation: true });
const ready = options => connectionReadiness({ support: supported, firmware: false, ...options });

test('a checked ZIP cannot enable a browser without Bluetooth', () => {
  const support = browserSupport({ secure: true, bluetooth: false, validation: true });
  const result = ready({ support, firmware: true });
  assert.equal(result.disabled, true);
  assert.equal(result.label, 'Bluetooth unavailable');
  assert.match(result.reason, /Web Bluetooth is not enabled or available/);
});

test('missing validation support is distinct from missing Bluetooth', () => {
  const support = browserSupport({ secure: true, bluetooth: true, validation: false });
  const result = ready({ support });
  assert.equal(result.disabled, true);
  assert.equal(result.label, 'Browser update required');
  assert.match(result.reason, /secure hashing and ZIP decompression/);
});

test('insecure pages explain HTTPS before absent secure-only APIs', () => {
  const support = browserSupport({ secure: false, bluetooth: false, validation: false });
  assert.equal(ready({ support }).label, 'Secure page required');
});

test('firmware selection and validation have distinct disabled explanations', () => {
  assert.equal(ready({}).title, 'Choose firmware first');
  const checking = ready({ activity: 'checking', firmware: true });
  assert.equal(checking.disabled, true);
  assert.equal(checking.label, 'Checking firmware…');
});

test('validation failure keeps its concrete reason next to the button', () => {
  const result = ready({ validationError: 'Image checksum does not match.' });
  assert.equal(result.disabled, true);
  assert.equal(result.title, 'Firmware checks failed');
  assert.equal(result.reason, 'Image checksum does not match.');
});

test('a valid package enables connection only after validation completes', () => {
  const result = ready({ firmware: true });
  assert.equal(result.disabled, false);
  assert.equal(result.label, 'Connect & update');
});

test('Bluetooth picker disables duplicate connection requests', () => {
  const result = ready({ activity: 'connecting', firmware: true });
  assert.equal(result.disabled, true);
  assert.equal(result.label, 'Connecting…');
});
