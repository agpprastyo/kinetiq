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
