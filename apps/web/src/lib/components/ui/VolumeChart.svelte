<script lang="ts">
  import type { VolumePoint } from "$lib/api";

  interface Props {
    points: VolumePoint[];
    height?: number;
    mode?: "bars" | "wave";
  }

  let { points = [], height = 200, mode = "bars" }: Props = $props();

  let hoveredPoint = $state<VolumePoint | null>(null);
  let hoveredIndex = $state<number | null>(null);

  // Sample down or bucket up to 8-12 clean pill bars for the Quixotic Pill-Bar aesthetic
  let barBuckets = $derived.by(() => {
    if (!points || points.length === 0) return [];
    const maxBars = 8;
    if (points.length <= maxBars) return points;
    const chunkSize = Math.ceil(points.length / maxBars);
    const result: VolumePoint[] = [];
    for (let i = 0; i < points.length; i += chunkSize) {
      const slice = points.slice(i, i + chunkSize);
      const sum = slice.reduce((acc, p) => acc + p.count, 0);
      result.push({
        timestamp: slice[0].timestamp || (slice[0] as any).bucket,
        count: sum,
      });
    }
    return result;
  });

  let maxBarCount = $derived(Math.max(...barBuckets.map((p) => p.count), 1));
  let peakBarIndex = $derived.by(() => {
    if (barBuckets.length === 0) return -1;
    let idx = 0;
    let maxVal = -1;
    barBuckets.forEach((b, i) => {
      if (b.count >= maxVal) {
        maxVal = b.count;
        idx = i;
      }
    });
    return idx;
  });

  let maxCount = $derived(Math.max(...points.map((p) => p.count), 1));
  let width = 800;

  let pathData = $derived(() => {
    if (points.length < 2) return { area: "", line: "", coords: [] };
    const step = width / (points.length - 1);
    const coords = points.map((p, i) => {
      const x = i * step;
      const y = height - (p.count / maxCount) * (height - 32) - 16;
      return { x, y };
    });

    let linePath = `M ${coords[0].x} ${coords[0].y}`;
    for (let i = 0; i < coords.length - 1; i++) {
      const curr = coords[i];
      const next = coords[i + 1];
      const mx = (curr.x + next.x) / 2;
      linePath += ` C ${mx} ${curr.y}, ${mx} ${next.y}, ${next.x} ${next.y}`;
    }

    const areaPath = `${linePath} L ${coords[coords.length - 1].x} ${height} L ${coords[0].x} ${height} Z`;
    return { area: areaPath, line: linePath, coords };
  });

  function formatTime(iso: string): string {
    try {
      const d = new Date(iso);
      return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    } catch {
      return iso;
    }
  }
</script>

<div class="relative w-full select-none">
  {#if points.length === 0}
    <div class="flex flex-col items-center justify-center h-44 text-[var(--text-muted)] text-xs">
      Belum ada trafik event pada rentang waktu ini
    </div>
  {:else if mode === "bars"}
    <!-- Quixotic Rounded Pill-Bar Chart with Striped Sage & Solid Peak Emerald Bar -->
    <div class="pt-7 pb-1 px-2">
      <div class="relative flex items-end justify-between gap-3 sm:gap-5" style="height: {height}px;">
        <!-- Subtle Horizontal Dashed Guide Lines -->
        <div class="absolute inset-0 flex flex-col justify-between pointer-events-none opacity-60">
          <div class="border-b border-dashed border-[var(--border-subtle)] w-full"></div>
          <div class="border-b border-dashed border-[var(--border-subtle)] w-full"></div>
          <div class="border-b border-dashed border-[var(--border-subtle)] w-full"></div>
          <div class="border-b border-[var(--border-subtle)] w-full"></div>
        </div>

        {#each barBuckets as bucket, i}
          {@const pct = Math.max(18, Math.round((bucket.count / maxBarCount) * 100))}
          {@const isPeak = i === peakBarIndex}
          {@const isHovered = hoveredIndex === i}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="relative flex-1 flex flex-col items-center justify-end h-full group cursor-pointer z-10"
            onmouseenter={() => {
              hoveredPoint = bucket;
              hoveredIndex = i;
            }}
            onmouseleave={() => {
              hoveredPoint = null;
              hoveredIndex = null;
            }}
          >
            <!-- Floating Pill Badge above Peak or Hovered Bar -->
            {#if isPeak || isHovered}
              <div class="absolute -top-7 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-[var(--primary-emerald)] text-white whitespace-nowrap bento-pill transition-transform">
                {bucket.count.toLocaleString()}
              </div>
              <div class="w-2.5 h-2.5 rounded-full bg-[var(--primary-emerald)] border-2 border-[var(--bg-surface)] mb-1 shrink-0"></div>
            {/if}

            <!-- Rounded Pill Vertical Bar -->
            <div
              class="w-full max-w-[46px] rounded-full transition-all duration-300 {isPeak || isHovered ? 'bg-[var(--primary-emerald)]' : 'sage-striped-bar opacity-85 hover:opacity-100'}"
              style="height: {pct}%;"
            ></div>

            <span class="mt-2.5 text-[10px] font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {formatTime(bucket.timestamp || (bucket as any).bucket)}
            </span>
          </div>
        {/each}
      </div>
    </div>
  {:else}
    <!-- Smooth Emerald Wave Area Sparkline (Quixotic Right Card Style) -->
    {@const computed = pathData()}
    <svg
      viewBox="0 0 {width} {height}"
      class="w-full overflow-visible"
      style="height: {height}px;"
      preserveAspectRatio="none"
    >
      <defs>
        <linearGradient id="emeraldWaveGradient" x1="0%" y1="0%" x2="0%" y2="100%">
          <stop offset="0%" stop-color="var(--primary-emerald)" stop-opacity="0.24" />
          <stop offset="100%" stop-color="var(--primary-emerald)" stop-opacity="0.0" />
        </linearGradient>
      </defs>

      <line x1="0" y1={height / 2} x2={width} y2={height / 2} stroke="var(--border-subtle)" stroke-dasharray="4 4" />

      {#if computed.coords.length >= 2}
        <path d={computed.area} fill="url(#emeraldWaveGradient)" />
        <path d={computed.line} fill="none" stroke="var(--primary-emerald)" stroke-width="2.5" stroke-linecap="round" />
      {/if}
    </svg>
  {/if}
</div>
