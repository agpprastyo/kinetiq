import { KinetiqConfig, IngestEvent } from './types';
import { StorageEngine, MemoryStorage, LocalStorageAdapter, IndexedDBStorage } from './storage';

function createEventId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `evt_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
}

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof document !== 'undefined';
}

function backoffWithJitter(baseMs: number, attempt: number): number {
  const exp = Math.min(baseMs * Math.pow(2, attempt), 10000);
  const jitter = Math.floor(Math.random() * Math.min(exp, 250));
  return exp + jitter;
}

export class KinetiqClient {
  private apiKey: string;
  private endpoint: string;
  private queue: IngestEvent[] = [];
  private flushTimer: ReturnType<typeof setInterval> | null = null;
  private maxBatchSize: number;
  private maxQueueSize: number;
  private currentUserId?: string;
  private storage: StorageEngine;
  private storageKey: string;
  private autoSyncOnOnline: boolean;
  private flushOnPageHide: boolean;
  private retryAttempt = 0;
  private baseRetryMs = 1000;
  private onlineHandler: (() => void) | null = null;
  private visibilityHandler: (() => void) | null = null;
  private pagehideHandler: (() => void) | null = null;
  private useMemoryQueue = false;

  constructor(config: KinetiqConfig) {
    this.apiKey = config.apiKey;
    this.endpoint = (config.endpoint || 'http://localhost:8080').replace(/\/$/, '');
    this.maxBatchSize = config.maxBatchSize || 50;
    this.maxQueueSize = config.maxQueueSize && config.maxQueueSize > 0 ? config.maxQueueSize : 1000;
    this.currentUserId = config.defaultUserId;
    this.storageKey = config.storageKey || 'kinetiq_telemetry_events';
    this.autoSyncOnOnline = config.autoSyncOnOnline !== false;
    this.flushOnPageHide = config.flushOnPageHide !== false;

    const preferred = config.storage;
    if (preferred === undefined) {
      this.storage = new MemoryStorage();
      this.useMemoryQueue = true;
    } else if (preferred === 'localstorage') {
      this.storage = new LocalStorageAdapter(this.storageKey);
    } else if (preferred === 'indexeddb' && isBrowser()) {
      this.storage = typeof indexedDB !== 'undefined' ? new IndexedDBStorage(this.storageKey) : new LocalStorageAdapter(this.storageKey);
    } else if (preferred === 'memory') {
      this.storage = new MemoryStorage();
      this.useMemoryQueue = true;
    } else {
      this.storage = new MemoryStorage();
      this.useMemoryQueue = true;
    }

    void this.storage.init();

    if (config.flushIntervalMs && config.flushIntervalMs > 0) {
      this.flushTimer = setInterval(() => {
        void this.flush();
      }, config.flushIntervalMs);
    }

    if (isBrowser() && typeof window !== 'undefined' && !this.useMemoryQueue) {
      if (this.autoSyncOnOnline && typeof window.addEventListener === 'function') {
        this.onlineHandler = () => { void this.flush(); };
        window.addEventListener('online', this.onlineHandler);
      }
      if (this.flushOnPageHide && typeof window.addEventListener === 'function') {
        this.visibilityHandler = () => {
          if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
            void this.flushKeepAlive();
          }
        };
        this.pagehideHandler = () => { void this.flushKeepAlive(); };
        if (typeof document !== 'undefined') {
          document.addEventListener('visibilitychange', this.visibilityHandler);
        }
        window.addEventListener('pagehide', this.pagehideHandler);
      }
    }
  }

  public identify(userId: string, traits?: Record<string, unknown>): void {
    this.currentUserId = userId;
    if (traits && Object.keys(traits).length > 0) {
      this.track('$identify', traits, userId);
    }
  }

  public track(eventName: string, properties?: Record<string, unknown>, userId?: string): void {
    const now = new Date().toISOString();
    const resolvedUser =
      userId ||
      (typeof properties?.distinct_id === 'string' ? properties.distinct_id : undefined) ||
      this.currentUserId;

    const evt: IngestEvent = {
      event_id: createEventId(),
      event_name: eventName,
      distinct_id: resolvedUser,
      user_id: resolvedUser,
      timestamp: now,
      occurred_at: now,
      properties: properties || {},
    };

    if (this.useMemoryQueue) {
      this.queue.push(evt);
      if (this.queue.length > this.maxQueueSize) {
        this.queue = this.queue.slice(this.queue.length - this.maxQueueSize);
      }
      if (this.queue.length >= this.maxBatchSize) {
        void this.flush();
      }
      return;
    }

    void this.enqueueWithCap(evt);

    void (async () => {
      try {
        const c = await this.storage.count();
        if (c >= this.maxBatchSize) {
          void this.flush();
        }
      } catch {}
    })();
  }

  private async enqueueWithCap(evt: IngestEvent): Promise<void> {
    try {
      await this.storage.enqueue([evt]);
      const c = await this.storage.count();
      if (c > this.maxQueueSize) {
        const excess = c - this.maxQueueSize;
        await this.storage.dequeue(excess);
      }
    } catch {}
  }

  public getQueueSize(): number {
    if (this.useMemoryQueue) {
      return this.queue.length;
    }
    return 0;
  }

  public async flush(): Promise<void> {
    if (this.useMemoryQueue) {
      if (this.queue.length === 0) return;
      const batch = [...this.queue];
      this.queue = [];

      const ingestUrl = this.endpoint.endsWith('/api/v1/ingest')
        ? this.endpoint
        : `${this.endpoint}/api/v1/ingest`;

      try {
        const response = await fetch(ingestUrl, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${this.apiKey}`,
          },
          body: JSON.stringify({ events: batch }),
        });

        if (!response.ok) {
          const isRetryable = response.status === 429 || response.status >= 500;
          if (isRetryable) {
            this.requeue(batch);
          }
        }
        return;
      } catch {
        this.requeue(batch);
        return;
      }
    }

    try {
      const count = await this.storage.count();
      if (count === 0) return;
      const batch = await this.storage.dequeue(Math.min(count, this.maxBatchSize));
      if (batch.length === 0) return;

      const ingestUrl = this.endpoint.endsWith('/api/v1/ingest')
        ? this.endpoint
        : `${this.endpoint}/api/v1/ingest`;

      try {
        const response = await fetch(ingestUrl, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${this.apiKey}`,
          },
          body: JSON.stringify({ events: batch }),
        });

        if (response.ok) {
          this.retryAttempt = 0;
          return;
        }

        const isRetryable = response.status === 429 || response.status >= 500;
        if (isRetryable) {
          await this.storage.requeue(batch);
          this.retryAttempt++;
          const delay = backoffWithJitter(this.baseRetryMs, this.retryAttempt);
          setTimeout(() => { void this.flush(); }, delay);
        }
      } catch {
        await this.storage.requeue(batch);
        this.retryAttempt++;
        const delay = backoffWithJitter(this.baseRetryMs, this.retryAttempt);
        setTimeout(() => { void this.flush(); }, delay);
      }
    } catch {}
  }

  private requeue(batch: IngestEvent[]): void {
    this.queue.unshift(...batch);
    if (this.queue.length > this.maxQueueSize) {
      this.queue = this.queue.slice(0, this.maxQueueSize);
    }
  }

  public flushKeepAlive(): void {
    if (!isBrowser()) return;
    void (async () => {
      try {
        let batch: IngestEvent[] = [];
        if (this.useMemoryQueue) {
          if (this.queue.length === 0) return;
          batch = this.queue.splice(0, Math.min(this.queue.length, this.maxBatchSize));
        } else {
          const count = await this.storage.count();
          if (count === 0) return;
          batch = await this.storage.dequeue(Math.min(count, this.maxBatchSize));
        }
        if (batch.length === 0) return;
        const ingestUrl = this.endpoint.endsWith('/api/v1/ingest')
          ? this.endpoint
          : `${this.endpoint}/api/v1/ingest`;
        const body = JSON.stringify({ events: batch });
        if (typeof navigator !== 'undefined' && typeof (navigator as any).sendBeacon === 'function') {
          const blob = new Blob([body], { type: 'application/json' });
          const sent = (navigator as any).sendBeacon(ingestUrl, blob);
          if (!sent) {
            try { await fetch(ingestUrl, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.apiKey}` }, body, keepalive: true }); }
            catch { if (this.useMemoryQueue) this.requeue(batch); else await this.storage.requeue(batch); }
          }
        } else {
          try {
            await fetch(ingestUrl, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.apiKey}` }, body, keepalive: true });
          } catch {
            if (this.useMemoryQueue) this.requeue(batch); else await this.storage.requeue(batch);
          }
        }
      } catch {}
    })();
  }

  public destroy(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
      this.flushTimer = null;
    }
    if (isBrowser() && typeof window !== 'undefined') {
      if (this.onlineHandler) window.removeEventListener('online', this.onlineHandler);
      if (this.visibilityHandler && typeof document !== 'undefined') document.removeEventListener('visibilitychange', this.visibilityHandler);
      if (this.pagehideHandler) window.removeEventListener('pagehide', this.pagehideHandler);
    }
  }
}

