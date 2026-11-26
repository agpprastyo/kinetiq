import { describe, it, expect } from 'bun:test';
import { funnelOptionsToQuery, dropoffPage } from '../src/lib/funnel';

describe('funnelOptionsToQuery', () => {
  it('is empty by default and omits ordered', () => {
    expect(funnelOptionsToQuery()).toBe('');
    expect(funnelOptionsToQuery({ order: 'ordered' })).toBe('');
  });
  it('encodes window and strict order', () => {
    expect(funnelOptionsToQuery({ window: '7d', order: 'strict' })).toBe('&window=7d&order=strict');
  });
});

describe('dropoffPage', () => {
  it('computes range and paging flags', () => {
    expect(dropoffPage(0, 50, 0)).toEqual({ from: 0, to: 0, hasPrev: false, hasNext: false });
    expect(dropoffPage(0, 50, 120)).toEqual({ from: 1, to: 50, hasPrev: false, hasNext: true });
    expect(dropoffPage(100, 50, 120)).toEqual({ from: 101, to: 120, hasPrev: true, hasNext: false });
  });
});
