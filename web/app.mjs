import { inspectPackage, MAX_PACKAGE_BYTES } from './firmware.mjs';
import { DirectUpdater, UpdateError, requestWatch } from './protocol.mjs';
import { browserSupport, connectionReadiness } from './support.mjs';

const $ = id => document.getElementById(id);
const supportsBluetooth = window.isSecureContext && typeof navigator.bluetooth?.requestDevice === 'function';
const supportsValidation = window.isSecureContext && Boolean(globalThis.DecompressionStream && globalThis.crypto?.subtle);
const support = browserSupport({ secure: window.isSecureContext, bluetooth: supportsBluetooth, validation: supportsValidation });
let firmware = null;
let release = null;
let updater = null;
let busy = false;
let transferring = false;
let paused = false;
let finished = false;
let wakeLock = null;
let activity = null;
let validationError = '';
let selectionRequested = false;

function error(message = '') {
  $('error').textContent = message;
  $('error').hidden = !message;
}
function status(message) { $('status').textContent = message; }
function step(number) {
  for (let n = 1; n <= 3; n++) {
    const item = $(`step-${n}`);
    item.classList.toggle('active', n === number);
    item.classList.toggle('complete', n < number);
    if (n === number) item.setAttribute('aria-current', 'step');
    else item.removeAttribute('aria-current');
  }
}
function controls() {
  const locked = busy || Boolean(updater);
  const readiness = connectionReadiness({ support, activity, firmware, validationError });
  $('file').disabled = locked || !supportsValidation;
  $('file-label').classList.toggle('disabled', $('file').disabled);
  $('latest').disabled = locked || !release || !supportsValidation;
  $('connect').disabled = busy || readiness.disabled;
  $('connect').textContent = readiness.label;
  $('connect').hidden = Boolean(updater);
  $('connection-notice').hidden = Boolean(updater);
  $('connection-notice').classList.toggle('unavailable', !support.available || Boolean(validationError));
  $('connection-title').textContent = readiness.title;
  $('connect-reason').textContent = readiness.reason;
  $('browser-handoff').hidden = support.available;
  $('linux-help').hidden = supportsBluetooth || !window.isSecureContext;
  $('pause').hidden = !transferring;
  $('pause').disabled = paused;
  $('resume').hidden = !paused || busy || finished;
  $('resume').disabled = Boolean(updater?.pendingOperations);
  $('reset').hidden = !updater || busy || finished;
  $('reset').disabled = Boolean(updater?.pendingOperations);
}
function progress(done, total) {
  const percentage = total ? Math.floor(done * 100 / total) : 0;
  $('progress').value = percentage;
  $('percent').textContent = `${percentage}%`;
  $('byte-count').textContent = `${done.toLocaleString()} / ${total.toLocaleString()} bytes confirmed by watch`;
  $('watch-fill').style.width = `${percentage}%`;
}
function onState({ phase, message }) {
  const labels = { connecting: 'Connecting', sending: 'Sending firmware', verifying: 'Checking on watch', reconnecting: 'Reconnecting', stopped: 'Transfer paused', ready: 'Verified on watch' };
  $('phase').textContent = labels[phase] || 'Updating';
  $('watch-label').textContent = labels[phase] || 'Updating…';
  if (!paused) status(message || labels[phase] || 'Updating…');
}
async function keepAwake() {
  if (!transferring || document.visibilityState !== 'visible' || !navigator.wakeLock) return;
  try {
    const lock = await navigator.wakeLock.request('screen');
    if (!transferring) { await lock.release(); return; }
    wakeLock = lock;
    lock.addEventListener('release', () => { if (wakeLock === lock) wakeLock = null; });
  } catch { /* Optional; the on-page instruction remains. */ }
}
async function releaseWakeLock() {
  const lock = wakeLock;
  wakeLock = null;
  try { await lock?.release(); } catch { /* Already released by the browser. */ }
}
function resetSelection() {
  firmware = null;
  $('file-name').textContent = 'No firmware selected';
  $('file-meta').textContent = 'A goPine OTA .zip package';
  $('validated').hidden = true;
  $('package-details').hidden = true;
}
async function selectPackage(bytes, name, expectedSha256) {
  resetSelection();
  const checked = await inspectPackage(bytes, { expectedSha256 });
  firmware = checked;
  $('file-name').textContent = name;
  $('file-meta').textContent = `goPine ${checked.version} · ${(checked.image.byteLength / 1024).toFixed(1)} KiB firmware`;
  $('validated').hidden = false;
  $('package-version').textContent = checked.version;
  $('package-size').textContent = `${bytes.byteLength.toLocaleString()} bytes in ZIP · ${checked.image.byteLength.toLocaleString()} bytes to send`;
  $('package-hash').textContent = checked.sha256;
  $('package-details').hidden = false;
  status(supportsBluetooth ? 'Package checked. Get your watch ready, then connect.' : 'Package checked. Open this page in a browser with Web Bluetooth to connect.');
}
// Opening the native picker already expresses a manual choice. The release
// lookup can finish while that picker is open, before a change event arrives.
$('file').addEventListener('click', () => {
  if (busy || updater) return;
  selectionRequested = true;
  if (!firmware) status('Choose a goPine OTA ZIP, or use the latest release.');
});
$('file').addEventListener('change', async event => {
  const file = event.target.files[0];
  if (!file || busy || updater) return;
  selectionRequested = true;
  busy = true;
  activity = 'checking';
  validationError = '';
  controls();
  error();
  status('Checking the firmware package…');
  try {
    if (file.size > MAX_PACKAGE_BYTES) throw new Error('This ZIP is too large for a goPine firmware package. Choose the application OTA ZIP from a goPine build.');
    await selectPackage(await file.arrayBuffer(), file.name);
  } catch (e) { resetSelection(); validationError = e.message; status('Choose a valid goPine firmware package.'); error(e.message); }
  finally { busy = false; activity = null; event.target.value = ''; controls(); }
});
async function downloadPackage(url) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 30000);
  try { return await readPackageDownload(url, controller.signal); }
  catch (e) {
    if (controller.signal.aborted) throw new Error('Firmware download timed out. Retry the latest release or choose a ZIP file.');
    throw e;
  } finally { clearTimeout(timeout); }
}
async function readPackageDownload(url, signal) {
  const response = await fetch(url, { cache: 'no-store', signal });
  if (!response.ok) throw new Error(`Firmware download failed (${response.status}). You can choose a downloaded ZIP instead.`);
  if (Number(response.headers.get('content-length')) > MAX_PACKAGE_BYTES) throw new Error('Firmware download exceeds the package size limit.');
  const reader = response.body.getReader();
  const chunks = [];
  let length = 0;
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.byteLength;
      if (length > MAX_PACKAGE_BYTES) throw new Error('Firmware download exceeds the package size limit.');
      chunks.push(value);
    }
  } catch (e) { await reader.cancel(); throw e; }
  finally { reader.releaseLock(); }
  const bytes = new Uint8Array(length);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  return bytes.buffer;
}
async function useLatestRelease() {
  if (!release || busy || updater || !supportsValidation) return;
  selectionRequested = true;
  busy = true;
  activity = 'checking';
  validationError = '';
  controls();
  error();
  status('Downloading and checking the published firmware…');
  try { await selectPackage(await downloadPackage(release.package), `goPine ${release.version} · published release`, release.sha256); }
  catch (e) { resetSelection(); validationError = e.message; status('Download could not be verified. Choose a ZIP to continue.'); error(e.message); }
  finally { busy = false; activity = null; controls(); }
}
$('latest').addEventListener('click', useLatestRelease);
async function transfer() {
  busy = true;
  transferring = true;
  paused = false;
  error();
  step(2);
  $('preparation').hidden = true;
  $('transfer-progress').hidden = false;
  $('transfer-title').textContent = 'Sending your update';
  controls();
  void keepAwake();
  try {
    await updater.run();
    if (paused) return;
    finished = true;
    step(3);
    progress(firmware.image.byteLength, firmware.image.byteLength);
    $('transfer-title').textContent = 'The rest is on your wrist';
    $('phase').textContent = 'Verified on watch';
    $('watch-label').textContent = 'Ready to install';
    status('Tap INSTALL on the watch. When goPine reboots, tap KEEP to retain this firmware.');
    $('transfer-help').textContent = 'Only confirm below after the watch has booted successfully and you have tapped KEEP.';
    $('confirmed').hidden = false;
    updater.disconnect();
  } catch (e) {
    if (paused || e.name === 'AbortError') {
      paused = true;
      status('Transfer paused. Leave the watch on its update screen, then resume here.');
    } else {
      const fatal = (e instanceof UpdateError && !e.retryable) || ['NotFoundError', 'NotSupportedError', 'SecurityError', 'TypeError'].includes(e.name);
      paused = !fatal;
      error(e.message || 'The Bluetooth transfer stopped. Check the watch and try again.');
      status(fatal ? 'Read the watch screen. Cancel the transfer there, then choose Start over here.' : 'Transfer paused. Keep the watch on its update screen and resume when ready.');
    }
    $('transfer-title').textContent = 'Transfer stopped';
  } finally {
    transferring = false;
    busy = false;
    await releaseWakeLock();
    controls();
  }
}
$('connect').addEventListener('click', async () => {
  if (busy || !firmware || updater || !supportsBluetooth) return;
  busy = true;
  activity = 'connecting';
  error();
  controls();
  status('Choose goPine Update in the Bluetooth picker.');
  try {
    // Keep requestDevice in this user gesture; firmware is already validated.
    const device = await requestWatch();
    updater = new DirectUpdater({ device, image: firmware.image, onProgress: progress, onState,
      onPendingChange: () => { if (!updater?.active) controls(); } });
    await transfer();
  } catch (e) {
    status('The watch is not connected. Open its update screen, then try again.');
    if (e.name !== 'NotFoundError') error(e.message || 'Bluetooth connection was not available.');
  } finally { busy = false; activity = null; controls(); }
});
$('updater-url').value = new URL('./', location.href).href;
$('copy-flag').addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText($('linux-flag').textContent);
    $('flag-copy-status').textContent = 'Copied. Paste this address into Chrome’s address bar.';
  } catch {
    $('flag-copy-status').textContent = 'Copy the chrome:// address shown above and paste it into Chrome’s address bar.';
  }
});
$('copy-link').addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText($('updater-url').value);
    $('copy-status').textContent = 'Link copied. Paste it into a supported browser to load the latest release.';
  } catch {
    $('updater-url').focus();
    $('updater-url').select();
    $('copy-status').textContent = 'Copy the selected link, then paste it into a supported browser.';
  }
});
$('pause').addEventListener('click', () => {
  if (!transferring || paused) return;
  paused = true;
  updater.stop();
  status('Pausing the transfer…');
  controls();
});
$('resume').addEventListener('click', () => { if (updater && !busy && paused && !finished && !updater.pendingOperations) void transfer(); });
$('reset').addEventListener('click', () => {
  if (busy || updater?.pendingOperations) return;
  updater?.disconnect();
  updater = null;
  paused = false;
  finished = false;
  progress(0, firmware?.image.byteLength || 0);
  $('preparation').hidden = false;
  $('transfer-progress').hidden = true;
  $('transfer-title').textContent = 'Get your watch ready';
  $('watch-label').textContent = "Ready for what's next.";
  step(1);
  error();
  status('Cancel the old transfer on the watch, then reopen Firmware Update before connecting again.');
  controls();
});
$('confirmed').addEventListener('click', () => {
  $('transfer-title').textContent = 'You’re all set';
  $('confirmed').hidden = true;
  $('watch-label').textContent = 'Hello again.';
  status(`goPine ${firmware.version} — boot and KEEP confirmed by you.`);
  $('transfer-help').textContent = 'You can close this tab. Enjoy your watch.';
});
window.addEventListener('beforeunload', event => {
  if (updater && !finished) { event.preventDefault(); event.returnValue = ''; }
});
window.addEventListener('pagehide', () => { updater?.stop(); void releaseWakeLock(); });
document.addEventListener('visibilitychange', () => { if (!wakeLock && transferring) void keepAwake(); });

async function loadRelease() {
  try {
    const response = await fetch('firmware/latest.json', { cache: 'no-store' });
    if (!response.ok) throw new Error('Release list unavailable.');
    const manifest = await response.json();
    if (manifest.schema !== 1) throw new Error('Release list is unsupported.');
    const candidate = manifest.firmware;
    if (!candidate) {
      $('release-description').textContent = 'No compatible firmware release is published yet. Choose an OTA ZIP from your goPine build.';
      if (!selectionRequested && !busy && !updater) status('Choose a goPine OTA ZIP to get started.');
      return;
    }
    if (!/^firmware\/gopine-dfu-\d+\.\d+\.\d+(?:\+\d+)?\.zip$/.test(candidate.package) || !/^[a-f0-9]{64}$/.test(candidate.sha256) || typeof candidate.version !== 'string' || candidate.version.length > 40) throw new Error('Release metadata is invalid.');
    release = candidate;
    $('latest').textContent = `Use release ${release.version}`;
    $('release-description').textContent = `goPine ${release.version} is the latest published release. You can also choose a local build.`;
    const releaseUrl = new URL(candidate.releaseUrl);
    if (releaseUrl.origin === 'https://github.com' && releaseUrl.pathname.startsWith('/Distortions81/goPineTime/releases/tag/')) {
      $('release-link').href = releaseUrl.href;
      $('release-link').hidden = false;
    }
    // A delayed release lookup must not replace a ZIP the user already chose.
    if (!selectionRequested && !firmware && !updater && !busy && supportsValidation) await useLatestRelease();
    else if (!supportsValidation && !selectionRequested) status('Open this page in a browser that can check firmware packages.');
  } catch {
    $('release-description').textContent = 'Published firmware is unavailable right now. You can still choose a goPine OTA ZIP.';
    if (!selectionRequested && !busy && !updater) status('Choose a goPine OTA ZIP to get started.');
  } finally { controls(); }
}
if (support.available) {
  $('browser-note').textContent = 'This browser supports Bluetooth updates.';
  $('browser-note').classList.add('supported');
} else {
  $('browser-note').textContent = support.title + '. See the connection notice below.';
  $('transfer-help').textContent = 'Safari and browsers on iPhone or iPad, including Chrome, do not support this updater’s Web Bluetooth connection.';
}
controls();
void loadRelease();
