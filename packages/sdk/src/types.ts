export type StorageType = 'indexeddb' | 'localstorage' | 'memory';

export interface KinetiqConfig {
  apiKey: string;
  endpoint?: string;
  flushIntervalMs?: number;
  maxBatchSize?: number;
  maxQueueSize?: number;
  defaultUserId?: string;

  // Fase 20: Persistence & Resilience
  storage?: StorageType;
  storageKey?: string;
  autoSyncOnOnline?: boolean;
  flushOnPageHide?: boolean;
  /** Abort a hung ingest request after this many ms and requeue. Default 10000. */
  requestTimeoutMs?: number;

  /** Web autocapture of $pageview/$pageleave + $session_id. Default true in browsers. */
  autocapture?: boolean;
  /** Inactivity gap that starts a new session. Default 30 minutes. */
  sessionTimeoutMs?: number;
  /** When the browser sends Do Not Track, capture nothing. Default true. */
  respectDoNotTrack?: boolean;
  /** Start opted out (e.g. until consent is given); call optIn() later. */
  optedOut?: boolean;
}

export type ClientConfig = KinetiqConfig;

export interface IngestEvent {
  event_id?: string;
  event_name: string;
  distinct_id?: string;
  user_id?: string;
  timestamp?: string;
  occurred_at?: string;
  properties?: Record<string, unknown>;
}
