// In-memory connection history only. Never include firmware data or watch IDs.
export function browserDescription(navigator) {
  const agent = navigator.userAgent || '';
  const match = agent.match(/(Edg)\/([\d.]+)/) || agent.match(/(Chrome|Firefox|Version)\/([\d.]+)/);
  const names = {Edg: 'Edge', Chrome: 'Chrome', Firefox: 'Firefox', Version: 'Safari'};
  const browser = match ? `${names[match[1]]} ${match[2]}` : 'Browser unavailable';
  const platform = /Android/.test(agent) ? 'Android' : /Linux/.test(agent) ? 'Linux' :
    /Windows/.test(agent) ? 'Windows' : /iPhone|iPad/.test(agent) ? 'iOS' : /Macintosh/.test(agent) ? 'macOS' : 'OS unavailable';
  return `${browser} on ${platform}`;
}

export class UpdateDiagnostics {
  #records = [];
  #secrets = [];
  #failure = null;
  #context = null;
  #now;
  constructor({now = () => new Date().toISOString()} = {}) { this.#now = now; }
  get active() { return this.#context !== null; }
  get records() { return this.#records.slice(); }
  get latestFailure() { return this.#failure; }
  clear() { this.#records = []; this.#secrets = []; this.#failure = null; this.#context = null; }
  start({version, browser}) {
    this.clear();
    this.#context = {version: this.#clean(version), browser: this.#clean(browser), started: this.#now()};
  }
  protect(...values) {
    this.#secrets.push(...values.filter(value => value !== undefined && value !== null && String(value).length > 0).map(String));
  }
  #clean(value) {
    let text = String(value ?? '').replace(/[\r\n\t]+/g, ' ');
    for (const secret of this.#secrets) text = text.split(secret).join('[redacted]');
    return text.replace(/\b(?:[a-f\d]{2}:){5}[a-f\d]{2}\b/gi, '[device address removed]')
      .replace(/\b[a-f\d]{8}-(?:[a-f\d]{4}-){3}[a-f\d]{12}\b/gi, '[identifier removed]')
      .replace(/\b(device (?:id|identifier)|session(?: token)?)\s*[:=]\s*[^\s,;]+/gi, '$1=[redacted]')
      .slice(0, 1000);
  }
  record(phase, message, {failure = false, code, stage} = {}) {
    if (!this.active) return false;
    const item = {time: this.#now(), phase: this.#clean(phase), message: this.#clean(message),
      code: this.#clean(code), stage: this.#clean(stage)};
    const previous = this.#records.at(-1);
    if (previous && ['phase', 'message', 'code', 'stage'].every(key => previous[key] === item[key])) return false;
    this.#records.push(item);
    if (this.#records.length > 20) this.#records.shift();
    if (failure) this.#failure = item;
    return true;
  }
  format(item) {
    const detail = [item.code, item.stage].filter(Boolean).join(' / ');
    return `${item.time} [${item.phase}]${detail ? ` (${detail})` : ''} ${item.message}`;
  }
  history() { return this.#records.map(item => this.format(item)).join('\n'); }
  report() {
    if (!this.active) return '';
    const {version, browser, started} = this.#context;
    return `goPine updater diagnostics\nSelected firmware: ${version}\nBrowser: ${browser}\nStarted: ${started}\n` +
      (this.#failure ? `Last failure: ${this.format(this.#failure)}\n` : '') +
      `\nLatest ${this.#records.length} connection events:\n${this.history()}`;
  }
}
