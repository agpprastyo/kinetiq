import { describe, it, expect } from 'bun:test';
import { formatDuration } from '../src/lib/format-duration';

describe('formatDuration', () => {
  it('formats seconds compactly', () => {
    expect(formatDuration(0)).toBe('0s');
    expect(formatDuration(45)).toBe('45s');
    expect(formatDuration(125)).toBe('2m 5s');
    expect(formatDuration(3725)).toBe('1h 2m');
  });
});
