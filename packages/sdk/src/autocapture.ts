const UTM_KEYS = ['utm_source', 'utm_medium', 'utm_campaign', 'utm_term', 'utm_content'];

export function parseCampaign(search: string): Record<string, string> {
  const params = new URLSearchParams(search);
  const out: Record<string, string> = {};
  for (const k of UTM_KEYS) {
    const v = params.get(k);
    if (v) out[k] = v;
  }
  return out;
}

export function referringDomain(referrer: string): string {
  try {
    return referrer ? new URL(referrer).hostname : '';
  } catch {
    return '';
  }
}

export interface AutocaptureEnv {
  window: any;
  document: any;
}

export type Capture = (eventName: string, properties: Record<string, unknown>) => void;

/** Emits $pageview (load + SPA navigation) and $pageleave (pagehide). Returns a stop function. */
export function startAutocapture(env: AutocaptureEnv, capture: Capture): () => void {
  const { window: win, document: doc } = env;
  let lastHref: string = win.location.href;
  let referrer: string = doc.referrer || '';

  const base = (): Record<string, unknown> => ({
    $current_url: win.location.href,
    $pathname: win.location.pathname,
    $title: doc.title,
  });

  const pageview = () => {
    const props: Record<string, unknown> = base();
    if (referrer) {
      props.$referrer = referrer;
      const domain = referringDomain(referrer);
      if (domain) props.$referring_domain = domain;
    }
    Object.assign(props, parseCampaign(new URL(win.location.href).search));
    capture('$pageview', props);
  };

  const onNavigate = () => {
    if (win.location.href === lastHref) return;
    referrer = lastHref;
    lastHref = win.location.href;
    pageview();
  };

  const history = win.history;
  const origPush = history.pushState;
  const origReplace = history.replaceState;
  const wrap = (orig: (...a: unknown[]) => unknown) =>
    function (this: unknown, ...args: unknown[]) {
      const r = orig.apply(history, args);
      onNavigate();
      return r;
    };
  const patchedPush = wrap(origPush);
  const patchedReplace = wrap(origReplace);
  history.pushState = patchedPush;
  history.replaceState = patchedReplace;

  const onPageHide = () => capture('$pageleave', base());
  win.addEventListener('popstate', onNavigate);
  win.addEventListener('hashchange', onNavigate);
  win.addEventListener('pagehide', onPageHide);

  pageview();

  return () => {
    if (history.pushState === patchedPush) history.pushState = origPush;
    if (history.replaceState === patchedReplace) history.replaceState = origReplace;
    win.removeEventListener('popstate', onNavigate);
    win.removeEventListener('hashchange', onNavigate);
    win.removeEventListener('pagehide', onPageHide);
  };
}
