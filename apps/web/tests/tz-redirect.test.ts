import { describe, it, expect } from 'bun:test';
import { extrasToQuery } from '../src/lib/filters';
import { loginUrl, safeRedirect } from '../src/lib/redirect';

describe('extrasToQuery tz', () => {
  it('always sends the browser IANA zone', () => {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(extrasToQuery()).toBe(`&tz=${encodeURIComponent(tz)}`);
    expect(extrasToQuery({ filter: 'f' })).toBe(`&filter=f&tz=${encodeURIComponent(tz)}`);
  });
});

describe('login redirect', () => {
  it('builds /login?redirect=<current>', () => {
    expect(loginUrl('/app/projects/1', '?x=1')).toBe('/login?redirect=%2Fapp%2Fprojects%2F1%3Fx%3D1');
  });
  it('accepts only same-origin paths', () => {
    expect(safeRedirect('/app/projects/1')).toBe('/app/projects/1');
    for (const bad of [null, '', 'https://evil.com', '//evil.com', '/\\evil.com', 'javascript:alert(1)']) {
      expect(safeRedirect(bad)).toBe('/app');
    }
  });
});
