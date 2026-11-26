export interface KeyValueStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

export const DEFAULT_SESSION_TIMEOUT_MS = 30 * 60 * 1000;
const STORAGE_KEY = 'kinetiq_session';

function newId(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return `ses_${Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')}`;
}

/** Rolling-inactivity session: id changes when the gap since last activity exceeds timeoutMs. */
export class SessionManager {
  private id = '';
  private last = 0;

  constructor(private timeoutMs = DEFAULT_SESSION_TIMEOUT_MS, private store?: KeyValueStore) {
    try {
      const raw = store?.getItem(STORAGE_KEY);
      if (raw) {
        const v = JSON.parse(raw);
        if (typeof v.id === 'string' && typeof v.last === 'number') {
          this.id = v.id;
          this.last = v.last;
        }
      }
    } catch {}
  }

  touch(now = Date.now()): string {
    if (!this.id || now - this.last > this.timeoutMs) this.id = newId();
    this.last = now;
    try { this.store?.setItem(STORAGE_KEY, JSON.stringify({ id: this.id, last: this.last })); } catch {}
    return this.id;
  }
}
