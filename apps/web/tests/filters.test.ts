import { describe, it, expect } from 'bun:test';
import { buildFilterParam, opsForType } from '../src/lib/filters';

describe('buildFilterParam', () => {
  it('returns empty string when there are no complete conditions', () => {
    expect(buildFilterParam([], 'and')).toBe('');
    expect(buildFilterParam([{ property: '', op: 'eq', value: 'x' }], 'and')).toBe('');
  });

  it('serializes a single condition without a group', () => {
    expect(JSON.parse(buildFilterParam([{ property: 'browser', op: 'eq', value: 'chrome' }], 'and'))).toEqual({
      property: 'browser', op: 'eq', value: 'chrome',
    });
  });

  it('wraps multiple conditions in an and/or group and coerces values', () => {
    const out = JSON.parse(buildFilterParam([
      { property: 'amount', op: 'gt', value: '10' },
      { property: 'browser', op: 'in', value: 'chrome, safari' },
      { property: 'plan', op: 'is_set', value: 'ignored' },
      { property: 'tier', op: 'eq', value: 'gold', scope: 'person' },
    ], 'or'));
    expect(out).toEqual({ group: 'or', filters: [
      { property: 'amount', op: 'gt', value: 10 },
      { property: 'browser', op: 'in', value: ['chrome', 'safari'] },
      { property: 'plan', op: 'is_set' },
      { property: 'tier', op: 'eq', value: 'gold', scope: 'person' },
    ] });
  });

  it('drops numeric conditions with non-numeric values', () => {
    expect(buildFilterParam([{ property: 'amount', op: 'lt', value: 'abc' }], 'and')).toBe('');
  });
});

describe('opsForType', () => {
  it('offers numeric comparisons only for numbers', () => {
    expect(opsForType('number')).toContain('gt');
    expect(opsForType('string')).not.toContain('gt');
    expect(opsForType('string')).toContain('contains');
    expect(opsForType('boolean')).toEqual(['eq', 'neq', 'is_set', 'is_not_set']);
  });
});
