export function loginUrl(pathname: string, search = ""): string {
  return `/login?redirect=${encodeURIComponent(pathname + search)}`;
}

/** Only same-origin absolute paths; anything else (open redirect, protocol) falls back to /app. */
export function safeRedirect(target: string | null | undefined): string {
  if (!target || !target.startsWith("/") || target.startsWith("//") || target.startsWith("/\\")) return "/app";
  return target;
}
