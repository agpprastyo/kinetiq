import { describe, it, expect, vi, beforeEach } from 'vitest';
import { KinetiqClient } from '../src/client';

describe('KinetiqClient', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('buffers events and flushes payload to endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'success', data: { ingested: 1 } }),
    });
    globalThis.fetch = fetchMock;

    const client = new KinetiqClient({
      apiKey: 'sp_live_test123',
      endpoint: 'http://localhost:8080',
      flushIntervalMs: 0, // Manual flush only for test
    });

    client.track('button_clicked', { button_id: 'hero_cta' });
    await client.flush();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, options] = fetchMock.mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/v1/ingest');
    expect(options.headers['Authorization']).toBe('Bearer sp_live_test123');
    
    const body = JSON.parse(options.body);
    expect(body.events).toHaveLength(1);
    expect(body.events[0].event_name).toBe('button_clicked');
    expect(body.events[0].properties).toEqual({ button_id: 'hero_cta' });
  });

  it('automatically flushes when maxBatchSize is reached', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'success', data: { ingested: 2 } }),
    });
    globalThis.fetch = fetchMock;

    const client = new KinetiqClient({
      apiKey: 'sp_live_test123',
      endpoint: 'http://localhost:8080',
      maxBatchSize: 2,
      flushIntervalMs: 0,
    });

    client.track('event_1');
    expect(fetchMock).not.toHaveBeenCalled();

    client.track('event_2');
    // Allow microtask to run
    await Promise.resolve();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('drops events on non-retryable 4xx errors (e.g. 400, 401, 403) to prevent poison batch loop', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
    });
    globalThis.fetch = fetchMock;

    const client = new KinetiqClient({
      apiKey: 'invalid_key',
      endpoint: 'http://localhost:8080',
      flushIntervalMs: 0,
    });

    client.track('event_poison');
    expect(client.getQueueSize()).toBe(1);

    await client.flush();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    // Queue should remain 0, discarded
    expect(client.getQueueSize()).toBe(0);
  });

  it('re-queues events on retryable 5xx or network errors and respects maxQueueSize', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
    });
    globalThis.fetch = fetchMock;

    const client = new KinetiqClient({
      apiKey: 'sp_live_test123',
      endpoint: 'http://localhost:8080',
      maxQueueSize: 3,
      flushIntervalMs: 0,
    });

    client.track('event_1');
    client.track('event_2');
    expect(client.getQueueSize()).toBe(2);

    await client.flush();

    // Re-queued because 503 is retryable
    expect(client.getQueueSize()).toBe(2);

    // Add more events up to and past maxQueueSize
    client.track('event_3');
    client.track('event_4');
    expect(client.getQueueSize()).toBe(3); // Capped at maxQueueSize 3
  });

  it('unload flush uses fetch keepalive with Authorization header (never header-less sendBeacon)', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 202 });
    globalThis.fetch = fetchMock;
    const beacon = vi.fn().mockReturnValue(true);
    (globalThis as any).navigator = { sendBeacon: beacon };

    const client = new KinetiqClient({ apiKey: 'sp_live_test123', endpoint: 'http://localhost:8080', flushIntervalMs: 0 });
    client.track('leaving');
    await client.flushKeepAlive();

    expect(beacon).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, options] = fetchMock.mock.calls[0];
    expect(options.keepalive).toBe(true);
    expect(options.headers['Authorization']).toBe('Bearer sp_live_test123');
    expect(JSON.parse(options.body).events[0].event_name).toBe('leaving');
    expect(client.getQueueSize()).toBe(0);
    delete (globalThis as any).navigator;
  });

  it('unload flush requeues the batch when the ingest rejects it with a retryable status', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({ ok: false, status: 503 });
    const client = new KinetiqClient({ apiKey: 'sp_live_test123', endpoint: 'http://localhost:8080', flushIntervalMs: 0 });
    client.track('leaving');
    await client.flushKeepAlive();
    expect(client.getQueueSize()).toBe(1);
  });

  it('aborts a hung flush after requestTimeoutMs and requeues the batch', async () => {
    let signal: AbortSignal | undefined;
    globalThis.fetch = vi.fn().mockImplementation((_url: string, opts: RequestInit) => {
      signal = opts.signal ?? undefined;
      return new Promise((_resolve, reject) => {
        opts.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
      });
    });
    const client = new KinetiqClient({ apiKey: 'sp_live_test123', endpoint: 'http://localhost:8080', flushIntervalMs: 0, requestTimeoutMs: 20 });
    client.track('slow');
    await client.flush();
    expect(signal?.aborted).toBe(true);
    expect(client.getQueueSize()).toBe(1);
  });
});
