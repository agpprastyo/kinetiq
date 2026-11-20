import type { IngestEvent } from "../types";
export interface StorageEngine {
  init(): Promise<void>;
  enqueue(events: IngestEvent[]): Promise<void>;
  dequeue(count: number): Promise<IngestEvent[]>;
  requeue(events: IngestEvent[]): Promise<void>;
  count(): Promise<number>;
  clear(): Promise<void>;
}


export class MemoryStorage implements StorageEngine {
  private queue: IngestEvent[] = [];
  async init() {}
  async enqueue(events: IngestEvent[]) { this.queue.push(...events); }
  async dequeue(count: number) { return this.queue.splice(0, count); }
  async requeue(events: IngestEvent[]) { this.queue.unshift(...events); }
  async count() { return this.queue.length; }
  async clear() { this.queue = []; }
}

export class LocalStorageAdapter implements StorageEngine {
  private key: string;
  constructor(key = 'kinetiq_telemetry_events') { this.key = key; }
  private read(): IngestEvent[] {
    try { return JSON.parse(localStorage.getItem(this.key) || '[]'); }
    catch { return []; }
  }
  private write(items: IngestEvent[]) {
    try { localStorage.setItem(this.key, JSON.stringify(items)); }
    catch { /* quota exceeded */ }
  }
  async init() {}
  async enqueue(events: IngestEvent[]) {
    const items = this.read();
    items.push(...events);
    this.write(items);
  }
  async dequeue(count: number) {
    const items = this.read();
    const out = items.splice(0, count);
    this.write(items);
    return out;
  }
  async requeue(events: IngestEvent[]) {
    const items = this.read();
    items.unshift(...events);
    this.write(items);
  }
  async count() { return this.read().length; }
  async clear() { try { localStorage.removeItem(this.key); } catch {} }
}

export class IndexedDBStorage implements StorageEngine {
  private key: string;
  private db: IDBDatabase | null = null;
  constructor(key = 'kinetiq_telemetry_events') { this.key = key; }
  private open(): Promise<IDBDatabase> {
    return new Promise((resolve, reject) => {
      if (typeof indexedDB === 'undefined') { reject(new Error('indexedDB unavailable')); return; }
      const req = indexedDB.open(this.key, 1);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains('events')) db.createObjectStore('events', { keyPath: 'seq', autoIncrement: true });
      };
      req.onsuccess = () => resolve(req.result);
      req.onerror = () => reject(req.error);
    });
  }
  async init() {
    try { this.db = await this.open(); } catch { this.db = null; }
  }
  private async all(): Promise<IngestEvent[]> {
    if (!this.db) return [];
    return new Promise((resolve) => {
      const tx = this.db!.transaction('events', 'readonly');
      const store = tx.objectStore('events');
      const req = store.getAll();
      req.onsuccess = () => resolve((req.result || []).map((r: any) => r.event));
      req.onerror = () => resolve([]);
    });
  }
  async enqueue(events: IngestEvent[]) {
    if (!this.db) return;
    const tx = this.db.transaction('events', 'readwrite');
    const store = tx.objectStore('events');
    for (const e of events) store.add({ event: e });
  }
  async dequeue(count: number) {
    if (!this.db) return [];
    const all = await this.all();
    const out = all.slice(0, count);
    // delete first count
    const tx = this.db.transaction('events', 'readwrite');
    const store = tx.objectStore('events');
    const req = store.openCursor();
    let n = 0;
    req.onsuccess = (e: any) => {
      const c = e.target.result;
      if (c && n < count) { c.delete(); n++; c.continue(); }
    };
    await new Promise((r) => { tx.oncomplete = () => r(null); tx.onerror = () => r(null); });
    return out;
  }
  async requeue(events: IngestEvent[]) {
    if (!this.db) return;
    const tx = this.db.transaction('events', 'readwrite');
    const store = tx.objectStore('events');
    for (const e of events.reverse()) store.add({ event: e });
    await new Promise((r) => { tx.oncomplete = () => r(null); tx.onerror = () => r(null); });
  }
  async count() { return (await this.all()).length; }
  async clear() {
    if (!this.db) return;
    const tx = this.db.transaction('events', 'readwrite');
    tx.objectStore('events').clear();
    await new Promise((r) => { tx.oncomplete = () => r(null); tx.onerror = () => r(null); });
  }
}
