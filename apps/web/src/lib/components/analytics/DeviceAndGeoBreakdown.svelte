<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Badge from "$lib/components/ui/Badge.svelte";
  import type { DeviceBreakdownItem, GeoBreakdownItem } from "$lib/api";
  import { Laptop, Smartphone, Tablet, Bot, HelpCircle, Globe } from "lucide-svelte";

  interface Props {
    devices: DeviceBreakdownItem[];
    geo: GeoBreakdownItem[];
    loading?: boolean;
  }

  let { devices = [], geo = [], loading = false }: Props = $props();

  const totalDeviceCount = $derived(
    devices.reduce((acc, curr) => acc + curr.count, 0) || 1
  );

  const totalGeoCount = $derived(
    geo.reduce((acc, curr) => acc + curr.count, 0) || 1
  );

  function getDeviceIcon(deviceType: string) {
    switch (deviceType.toLowerCase()) {
      case "desktop":
        return Laptop;
      case "mobile":
        return Smartphone;
      case "tablet":
        return Tablet;
      case "bot":
        return Bot;
      default:
        return HelpCircle;
    }
  }

  function getDeviceColor(deviceType: string): string {
    switch (deviceType.toLowerCase()) {
      case "desktop":
        return "bg-indigo-500 text-indigo-400";
      case "mobile":
        return "bg-emerald-500 text-emerald-400";
      case "tablet":
        return "bg-amber-500 text-amber-400";
      case "bot":
        return "bg-rose-500 text-rose-400";
      default:
        return "bg-zinc-500 text-zinc-400";
    }
  }

  function getCountryFlag(countryCode: string): string {
    if (!countryCode || countryCode === "unknown" || countryCode === "LOCAL") {
      return "🌐";
    }
    const code = countryCode.toUpperCase();
    if (code.length !== 2) return "🌐";
    // Convert 2-letter ISO code to Regional Indicator Symbol letters
    const charCode1 = code.charCodeAt(0) - 65 + 0x1f1e6;
    const charCode2 = code.charCodeAt(1) - 65 + 0x1f1e6;
    return String.fromCodePoint(charCode1, charCode2);
  }
</script>

<div class="grid grid-cols-1 lg:grid-cols-2 gap-5 pt-2">
  <!-- 1. Device Breakdown Bento Card -->
  <Card class="space-y-4">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2.5">
        <div class="w-9 h-9 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
          <Laptop class="w-4 h-4" />
        </div>
        <div>
          <h3 class="text-sm font-bold text-[var(--text-primary)]">Device Distribution</h3>
          <p class="text-[11px] text-[var(--text-muted)]">Breakdown by client device type</p>
        </div>
      </div>
      <Badge variant="neutral">Auto-enriched</Badge>
    </div>

    <!-- Multi-segment progress bar -->
    {#if !loading && devices.length > 0}
      <div class="h-2.5 w-full bg-[var(--bg-surface-muted)] rounded-full overflow-hidden flex">
        {#each devices as dev}
          {@const pct = (dev.count / totalDeviceCount) * 100}
          <div
            style="width: {pct}%"
            class="{getDeviceColor(dev.device_type).split(' ')[0]} h-full transition-all duration-500"
            title="{dev.device_type}: {dev.count} ({pct.toFixed(1)}%)"
          ></div>
        {/each}
      </div>
    {/if}

    <div class="space-y-2">
      {#if loading}
        <div class="py-6 text-center text-xs text-[var(--text-muted)]">
          <div class="w-5 h-5 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin mx-auto mb-2"></div>
          Loading device breakdown...
        </div>
      {:else if devices.length === 0}
        <div class="py-6 text-center text-xs text-[var(--text-muted)]">
          No device data recorded in this period.
        </div>
      {:else}
        {#each devices as dev}
          {@const pct = (dev.count / totalDeviceCount) * 100}
          {@const Icon = getDeviceIcon(dev.device_type)}
          <div class="flex items-center justify-between p-2.5 rounded-xl bg-[var(--bg-surface-muted)]/60 text-xs">
            <div class="flex items-center gap-2.5">
              <div class="w-7 h-7 rounded-lg bg-[var(--bg-canvas)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--text-secondary)]">
                <Icon class="w-3.5 h-3.5" />
              </div>
              <div>
                <span class="font-semibold capitalize text-[var(--text-primary)]">{dev.device_type}</span>
                <span class="text-[10px] text-[var(--text-muted)] ml-1.5">({pct.toFixed(1)}%)</span>
              </div>
            </div>
            <span class="font-mono font-bold text-[var(--text-primary)]">{dev.count.toLocaleString()}</span>
          </div>
        {/each}
      {/if}
    </div>
  </Card>

  <!-- 2. Top Locations Bento Card -->
  <Card class="space-y-4">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2.5">
        <div class="w-9 h-9 rounded-full bg-[var(--brand-soft)] text-[var(--brand-primary)] flex items-center justify-center">
          <Globe class="w-4 h-4" />
        </div>
        <div>
          <h3 class="text-sm font-bold text-[var(--text-primary)]">Top Locations</h3>
          <p class="text-[11px] text-[var(--text-muted)]">Audience origin by ISO country</p>
        </div>
      </div>
      <Badge variant="neutral">Privacy-safe</Badge>
    </div>

    <div class="space-y-2">
      {#if loading}
        <div class="py-6 text-center text-xs text-[var(--text-muted)]">
          <div class="w-5 h-5 rounded-full border-2 border-emerald-500/20 border-t-[var(--brand-primary)] animate-spin mx-auto mb-2"></div>
          Loading locations...
        </div>
      {:else if geo.length === 0}
        <div class="py-6 text-center text-xs text-[var(--text-muted)]">
          No location data recorded in this period.
        </div>
      {:else}
        {#each geo as g}
          {@const pct = (g.count / totalGeoCount) * 100}
          <div class="space-y-1">
            <div class="flex items-center justify-between text-xs">
              <div class="flex items-center gap-2">
                <span class="text-base leading-none">{getCountryFlag(g.country_code)}</span>
                <span class="font-mono font-semibold text-[var(--text-primary)] uppercase">{g.country_code}</span>
                {#if g.country_code === "LOCAL"}
                  <span class="text-[10px] text-[var(--text-muted)] font-sans">(Localhost / Dev)</span>
                {/if}
              </div>
              <div class="flex items-center gap-2 font-mono">
                <span class="text-[10px] text-[var(--text-muted)] font-sans">{pct.toFixed(1)}%</span>
                <span class="font-bold text-[var(--text-primary)]">{g.count.toLocaleString()}</span>
              </div>
            </div>
            <!-- Progress bar -->
            <div class="h-1.5 w-full bg-[var(--bg-surface-muted)] rounded-full overflow-hidden">
              <div style="width: {pct}%" class="h-full bg-[var(--brand-primary)] rounded-full"></div>
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </Card>
</div>
