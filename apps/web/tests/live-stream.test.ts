import { describe, it, expect, mock } from 'bun:test';
import { connectLiveStream } from '../src/lib/live-stream';

describe('connectLiveStream', () => {
  it('subscribes to EventSource and triggers onEvent callback', () => {
    const onEvent = mock(() => {});
    const mockEventSource = {
      onmessage: null as any,
      close: mock(() => {}),
    };
    (globalThis as any).EventSource = mock(() => mockEventSource);

    const disconnect = connectLiveStream('proj-123', onEvent);

    mockEventSource.onmessage({
      data: JSON.stringify({ name: 'checkout_completed', event_id: 'evt-1' }),
    });

    expect(onEvent).toHaveBeenCalledTimes(1);
    expect(onEvent).toHaveBeenCalledWith({
      name: 'checkout_completed',
      event_id: 'evt-1',
    });

    disconnect();
    expect(mockEventSource.close).toHaveBeenCalledTimes(1);
  });
});
