export function browserSupport({ secure, bluetooth, validation }) {
  if (!secure) return {
    available: false,
    label: 'Secure page required',
    title: 'Open the updater over HTTPS',
    reason: 'Bluetooth and firmware checks require a secure page. Open the published updater with an https:// address.',
  };
  if (!bluetooth) return {
    available: false,
    label: 'Bluetooth unavailable',
    title: 'Bluetooth is unavailable in this browser',
    reason: 'Web Bluetooth is not enabled or available here. For Chrome on Linux, follow the setup below. Other supported options include Chrome or Edge on a supported computer, or Chrome on Android. If you are viewing this page inside an app, copy its link into a supported browser.',
  };
  if (!validation) return {
    available: false,
    label: 'Browser update required',
    title: 'This browser cannot check firmware packages',
    reason: 'Firmware checks require secure hashing and ZIP decompression. Update Chrome or Edge, then reopen this page before choosing a package.',
  };
  return { available: true };
}

export function connectionReadiness({ support, activity, firmware, validationError }) {
  if (!support.available) return { ...support, disabled: true };
  if (activity === 'checking') return {
    disabled: true, label: 'Checking firmware…', title: 'Checking the package',
    reason: 'Connect & update becomes available after the firmware checks pass.',
  };
  if (activity === 'connecting') return {
    disabled: true, label: 'Connecting…', title: 'Choose your watch',
    reason: 'Select goPine Update in the browser’s Bluetooth picker.',
  };
  if (validationError) return {
    disabled: true, label: 'Choose a valid package', title: 'Firmware checks failed',
    reason: validationError,
  };
  if (!firmware) return {
    disabled: true, label: 'Connect & update', title: 'Choose firmware first',
    reason: 'Select a published release or choose a goPine OTA ZIP. The connection button becomes available when its checks pass.',
  };
  return {
    disabled: false, label: 'Connect & update', title: 'Ready to connect',
    reason: 'Package checked. Open Firmware Update on the watch, then connect below.',
  };
}
