import { describe, it, expect, vi, afterEach } from 'vitest';
import { SessionManager } from '../src/session';
import { parseCampaign, referringDomain, startAutocapture } from '../src/autocapture';
import { KinetiqClient } from '../src/client';

function fakeWindow(href: string, referrer = '', dnt: string | null = null) {
  const listeners: Record<string, Array<(e?: unknown) => void>> = {};
  const store: Record<string, string> = {};
  const loc = { href, pathname: new URL(href).pathname };
  const setUrl = (u: string) => {
    const url = new URL(u, loc.href);
    loc.href = url.href;
    loc.pathname = url.pathname;
  };
  const win: any = {
    location: loc,
    history: {
      pushState: (_s: unknown, _t: string, u?: string) => u && setUrl(u),
      replaceState: (_s: unknown, _t: string, u?: string) => u && setUrl(u),
    },
    navigator: { doNotTrack: dnt },
    sessionStorage: {
      getItem: (k: string) => store[k] ?? null,
      setItem: (k: string, v: string) => { store[k] = v; },
    },
    addEventListener: (n: string, fn: () => void) => { (listeners[n] ||= []).push(fn); },
    removeEventListener: (n: string, fn: () => void) => { listeners[n] = (listeners[n] || []).filter((f) => f !== fn); },
    fire: (n: string) => (listeners[n] || []).forEach((f) => f()),
  };
  const doc: any = { title: 'Home', referrer };
  return { win, doc };
}

describe('parseCampaign / referringDomain', () => {
  it('extracts only utm_* params', () => {
    expect(parseCampaign('?utm_source=g&utm_medium=cpc&utm_campaign=x&utm_term=t&utm_content=c&foo=1')).toEqual({
      utm_source: 'g', utm_medium: 'cpc', utm_campaign: 'x', utm_term: 't', utm_content: 'c',
    });
    expect(parseCampaign('')).toEqual({});
  });
  it('derives referring domain', () => {
    expect(referringDomain('https://www.google.com/search?q=1')).toBe('www.google.com');
    expect(referringDomain('')).toBe('');
    expect(referringDomain('garbage')).toBe('');
  });
});

describe('SessionManager', () => {
  it('reuses id within timeout and rotates after gap', () => {
    const sm = new SessionManager(30 * 60_000);
    const a = sm.touch(0);
    expect(sm.touch(29 * 60_000)).toBe(a);
    expect(sm.touch(29 * 60_000 + 30 * 60_000)).toBe(a); // gap measured from last activity
    const b = sm.touch(29 * 60_000 + 30 * 60_000 + 30 * 60_000 + 1);
    expect(b).not.toBe(a);
  });
  it('persists across instances via storage', () => {
    const store: Record<string, string> = {};
    const s = { getItem: (k: string) => store[k] ?? null, setItem: (k: string, v: string) => { store[k] = v; } };
    const id = new SessionManager(1000, s).touch(0);
    expect(new SessionManager(1000, s).touch(500)).toBe(id);
  });
});

describe('startAutocapture', () => {
  it('emits $pageview on load with url, path, title, referrer, utm', () => {
    const { win, doc } = fakeWindow('https://a.com/landing?utm_source=news&utm_medium=email', 'https://t.co/x');
    const capture = vi.fn();
    startAutocapture({ window: win, document: doc }, capture);
    expect(capture).toHaveBeenCalledWith('$pageview', {
      $current_url: 'https://a.com/landing?utm_source=news&utm_medium=email',
      $pathname: '/landing',
      $title: 'Home',
      $referrer: 'https://t.co/x',
      $referring_domain: 't.co',
      utm_source: 'news',
      utm_medium: 'email',
    });
  });

  it('emits $pageview on SPA pushState/replaceState/popstate, once per URL change', () => {
    const { win, doc } = fakeWindow('https://a.com/');
    const capture = vi.fn();
    startAutocapture({ window: win, document: doc }, capture);
    capture.mockClear();
    win.history.pushState({}, '', '/pricing');
    expect(capture).toHaveBeenCalledTimes(1);
    expect(capture.mock.calls[0][1]).toMatchObject({ $pathname: '/pricing', $referrer: 'https://a.com/' });
    win.history.replaceState({}, '', '/pricing'); // same URL: no event
    expect(capture).toHaveBeenCalledTimes(1);
    win.history.replaceState({}, '', '/docs');
    expect(capture).toHaveBeenCalledTimes(2);
    win.location.href = 'https://a.com/back';
    win.location.pathname = '/back';
    win.fire('popstate');
    expect(capture).toHaveBeenCalledTimes(3);
  });

  it('emits $pageleave on pagehide', () => {
    const { win, doc } = fakeWindow('https://a.com/x');
    const capture = vi.fn();
    startAutocapture({ window: win, document: doc }, capture);
    capture.mockClear();
    win.fire('pagehide');
    expect(capture).toHaveBeenCalledWith('$pageleave', expect.objectContaining({ $pathname: '/x', $current_url: 'https://a.com/x' }));
  });

  it('stop() restores history and removes listeners', () => {
    const { win, doc } = fakeWindow('https://a.com/');
    const orig = win.history.pushState;
    const capture = vi.fn();
    const stop = startAutocapture({ window: win, document: doc }, capture);
    stop();
    capture.mockClear();
    expect(win.history.pushState).toBe(orig);
    win.fire('pagehide');
    expect(capture).not.toHaveBeenCalled();
  });
});

describe('KinetiqClient autocapture', () => {
  afterEach(() => {
    delete (globalThis as any).window;
    delete (globalThis as any).document;
  });

  function setup(config: Record<string, unknown> = {}, dnt: string | null = null) {
    const { win, doc } = fakeWindow('https://a.com/home?utm_source=x', 'https://google.com/', dnt);
    (globalThis as any).window = win;
    (globalThis as any).document = { ...doc, addEventListener: () => {}, removeEventListener: () => {} };
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    globalThis.fetch = fetchMock as any;
    const client = new KinetiqClient({ apiKey: 'k', flushIntervalMs: 0, ...config });
    return { win, client, fetchMock };
  }

  async function sent(fetchMock: ReturnType<typeof vi.fn>) {
    return fetchMock.mock.calls.flatMap((c) => JSON.parse(c[1].body).events) as any[];
  }

  it('captures $pageview with a shared $session_id by default', async () => {
    const { win, client, fetchMock } = setup();
    win.history.pushState({}, '', '/next');
    client.track('click');
    await client.flush();
    const events = await sent(fetchMock);
    expect(events.map((e) => e.event_name)).toEqual(['$pageview', '$pageview', 'click']);
    expect(events[0].properties.utm_source).toBe('x');
    const ids = new Set(events.map((e) => e.properties.$session_id));
    expect(ids.size).toBe(1);
    expect([...ids][0]).toBeTruthy();
    client.destroy();
  });

  it('autocapture:false disables pageviews and session ids', async () => {
    const { win, client, fetchMock } = setup({ autocapture: false });
    win.history.pushState({}, '', '/next');
    client.track('click');
    await client.flush();
    const events = await sent(fetchMock);
    expect(events).toHaveLength(1);
    expect(events[0].properties.$session_id).toBeUndefined();
    client.destroy();
  });

  it('respects Do Not Track unless respectDoNotTrack:false', async () => {
    const a = setup({}, '1');
    expect(a.client.getQueueSize()).toBe(0);
    a.client.destroy();
    const b = setup({ respectDoNotTrack: false }, '1');
    expect(b.client.getQueueSize()).toBe(1);
    b.client.destroy();
  });

  it('optOut() stops capture and optIn() resumes', () => {
    const { win, client } = setup();
    expect(client.getQueueSize()).toBe(1);
    client.optOut();
    win.history.pushState({}, '', '/a');
    expect(client.getQueueSize()).toBe(1);
    client.optIn();
    expect(client.getQueueSize()).toBe(2); // fresh $pageview on opt-in
    client.destroy();
  });

  it('new session after 30-min inactivity (configurable)', async () => {
    const now = vi.spyOn(Date, 'now').mockReturnValue(0);
    try {
      const { client, fetchMock } = setup({ sessionTimeoutMs: 60_000 });
      now.mockReturnValue(61_001);
      client.track('later');
      await client.flush();
      const events = await sent(fetchMock);
      expect(events[0].properties.$session_id).not.toBe(events[1].properties.$session_id);
      client.destroy();
    } finally {
      now.mockRestore();
    }
  });
});
