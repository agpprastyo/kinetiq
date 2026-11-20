const AUTH_BASE_URL = import.meta.env.VITE_AUTH_URL || "http://localhost:3001";

export interface SetupPayload {
  name: string;
  email: string;
  password: string;
  workspaceName: string;
  appUrl: string;
  timezone: string;
}

/** Returns null when status can't be determined (fail open: never block login on a network error). */
export async function getSetupStatus(): Promise<boolean | null> {
  try {
    const res = await fetch(`${AUTH_BASE_URL}/api/v1/setup/status`, { credentials: "include" });
    if (!res.ok) return null;
    return Boolean((await res.json()).initialized);
  } catch {
    return null;
  }
}

export async function submitSetup(payload: SetupPayload): Promise<{ ok: true } | { ok: false; status: number; message: string }> {
  const res = await fetch(`${AUTH_BASE_URL}/api/v1/setup`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (res.ok) return { ok: true };
  const body = await res.json().catch(() => null);
  return { ok: false, status: res.status, message: body?.error?.message || "Setup failed." };
}
