export type ComponentHealth = "healthy" | "degraded" | "down";

export function healthBadgeClasses(status: string): string {
  if (status === "healthy") {
    return "bg-emerald-500/15 text-emerald-500 border-emerald-500/30";
  }
  if (status === "degraded") {
    return "bg-amber-500/15 text-amber-500 border-amber-500/30";
  }
  return "bg-red-500/15 text-red-500 border-red-500/30";
}

export function queueHealth(pendingPelCount: number, consumerLag: number): ComponentHealth {
  if (pendingPelCount > 1000 || consumerLag > 5000) {
    return "degraded";
  }
  return "healthy";
}
