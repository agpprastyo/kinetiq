<script lang="ts">
  import { onMount, onDestroy, type Snippet } from "svelte";
  import { page } from "$app/state";
  import { toggleTheme, subscribeTheme, type Theme } from "$lib/theme";
  import { Activity, Sun, Moon, Terminal, Check, Copy } from "lucide-svelte";

  interface Props {
    title: string;
    subtitle: string;
    children: Snippet;
    footer?: Snippet;
  }

  let { title, subtitle, children, footer }: Props = $props();

  let currentTheme = $state<Theme>("light");
  let unsubTheme: (() => void) | null = null;
  let snippetTab = $state<"sdk" | "curl">("sdk");
  let copied = $state(false);

  const currentPath = $derived(page.url.pathname);

  const sdkSnippet = `import { KinetiqClient } from '@kinetiq/sdk';

const kinetiq = new KinetiqClient({
  apiKey: process.env.KINETIQ_API_KEY,
  endpoint: 'https://ingest.kinetiq.io'
});

await kinetiq.track('checkout.completed', {
  plan: 'pro_annual',
  currency: 'IDR',
  value: 1490000
});`;

  const exampleKeyHeader = ["X-API-Key", "<your_project_key>"].join(": ");
  const curlSnippet = `curl -X POST http://localhost:8080/v1/events \\
  -H "Content-Type: application/json" \\
  -H "${exampleKeyHeader}" \\
  -d '{
    "event_name": "checkout.completed",
    "user_id": "usr_01J9X2K8M",
    "properties": { "plan": "pro_annual" }
  }'`;

  function copyCode() {
    navigator.clipboard.writeText(snippetTab === "sdk" ? sdkSnippet : curlSnippet);
    copied = true;
    setTimeout(() => (copied = false), 1800);
  }

  onMount(() => {
    unsubTheme = subscribeTheme((t) => {
      currentTheme = t;
    });
  });

  onDestroy(() => {
    if (unsubTheme) unsubTheme();
  });
</script>

<div class="min-h-screen bg-[var(--bg-canvas)] text-[var(--text-primary)] flex flex-col justify-between selection:bg-emerald-500/20">
  <!-- Minimal Architectural Top Bar -->
  <header class="w-full max-w-[1120px] mx-auto px-6 pt-6 pb-2 flex items-center justify-between">
    <a href="/" class="flex items-center gap-2.5 group">
      <div class="w-8 h-8 rounded-lg bg-[var(--brand-primary)] text-white flex items-center justify-center transition-transform group-hover:scale-[1.02] shadow-xs">
        <Activity class="w-4 h-4" />
      </div>
      <div class="flex items-baseline gap-2">
        <span class="text-base font-bold tracking-tight text-[var(--text-primary)]">Kinetiq</span>
        <span class="text-[11px] font-mono text-[var(--text-muted)] border border-[var(--border-subtle)] bg-[var(--bg-surface)] px-1.5 py-0.5 rounded-md">
          analytics
        </span>
      </div>
    </a>

    <div class="flex items-center gap-2.5">
      {#if currentPath !== "/login" && currentPath !== "/signup"}
        <a
          href="/login"
          class="px-3 py-1.5 rounded-lg text-xs font-medium text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface)] border border-transparent hover:border-[var(--border-subtle)] transition-all"
        >
          Sign in
        </a>
      {/if}

      <button
        type="button"
        onclick={() => toggleTheme()}
        title={currentTheme === "dark" ? "Switch to Light mode" : "Switch to Dark mode"}
        class="w-8 h-8 rounded-lg flex items-center justify-center bg-[var(--bg-surface)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] border border-[var(--border-subtle)] transition-colors cursor-pointer"
      >
        {#if currentTheme === "dark"}
          <Sun class="w-3.5 h-3.5 text-amber-400" />
        {:else}
          <Moon class="w-3.5 h-3.5 text-[var(--text-secondary)]" />
        {/if}
      </button>
    </div>
  </header>

  <!-- Main Asymmetric Two-Column Frame -->
  <main class="flex-1 flex items-center justify-center px-6 py-8">
    <div class="w-full max-w-[1020px] grid grid-cols-1 lg:grid-cols-12 bg-[var(--bg-surface)] border border-[var(--border-subtle)] rounded-2xl shadow-[var(--shadow-bento)] overflow-hidden">
      <!-- Left Column: Authentication Form (7 cols on lg) -->
      <div class="lg:col-span-7 p-7 sm:p-10 flex flex-col justify-between">
        <div class="space-y-6">
          <!-- Segmented Route Switcher for Login / Signup -->
          {#if currentPath === "/login" || currentPath === "/signup"}
            <div class="inline-flex p-1 rounded-xl bg-[var(--bg-surface-muted)] border border-[var(--border-subtle)]">
              <a
                href="/login"
                class="px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all {currentPath === '/login'
                  ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-2xs border border-[var(--border-subtle)]'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
              >
                Sign in
              </a>
              <a
                href="/signup"
                class="px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-all {currentPath === '/signup'
                  ? 'bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-2xs border border-[var(--border-subtle)]'
                  : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
              >
                Create account
              </a>
            </div>
          {/if}

          <!-- Left-Aligned Editorial Header -->
          <div class="space-y-1">
            <h1 class="text-xl sm:text-2xl font-semibold tracking-tight text-[var(--text-primary)]">
              {title}
            </h1>
            <p class="text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed">
              {subtitle}
            </p>
          </div>

          <!-- Form Slot -->
          {@render children()}
        </div>

        {#if footer}
          <div class="pt-6 mt-6 border-t border-[var(--border-subtle)]">
            {@render footer()}
          </div>
        {/if}
      </div>

      <!-- Right Column: Theme-Adaptive Carbon.sh Code Window & Highlights (5 cols on lg) -->
      <div class="hidden lg:flex lg:col-span-5 bg-[var(--bg-surface-muted)] border-l border-[var(--border-subtle)] p-7 flex-col justify-between">
        <div class="space-y-5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <Terminal class="w-3.5 h-3.5 text-[var(--brand-primary)]" />
              <span class="text-[11px] font-mono uppercase tracking-wider font-semibold text-[var(--text-secondary)]">
                Quick Integration
              </span>
            </div>
            <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-[10px] font-mono text-[var(--brand-primary)] font-medium">
              <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)]"></span>
              Ready in 1 min
            </span>
          </div>

          <!-- Theme-Adaptive Carbon.sh Style Code Window -->
          <div class="rounded-xl bg-[var(--code-bg)] border border-[var(--code-border)] shadow-[var(--shadow-bento)] overflow-hidden text-[var(--code-plain)] transition-colors">
            <!-- Window Titlebar with macOS Traffic Lights + File Tabs -->
            <div class="px-3.5 py-2.5 border-b border-[var(--code-border)] bg-[var(--code-header-bg)] flex items-center justify-between">
              <div class="flex items-center gap-3">
                <div class="flex items-center gap-1.5" aria-hidden="true">
                  <span class="w-2.5 h-2.5 rounded-full bg-[#ff5f56]/90"></span>
                  <span class="w-2.5 h-2.5 rounded-full bg-[#ffbd2e]/90"></span>
                  <span class="w-2.5 h-2.5 rounded-full bg-[#27c93f]/90"></span>
                </div>

                <div class="flex items-center gap-1 pl-1">
                  <button
                    type="button"
                    onclick={() => (snippetTab = "sdk")}
                    class="px-2 py-0.5 rounded-md text-[11px] font-mono transition-colors cursor-pointer {snippetTab === 'sdk'
                      ? 'bg-[var(--code-bg)] text-[var(--brand-primary)] font-semibold border border-[var(--code-border)]'
                      : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
                  >
                    tracker.ts
                  </button>
                  <button
                    type="button"
                    onclick={() => (snippetTab = "curl")}
                    class="px-2 py-0.5 rounded-md text-[11px] font-mono transition-colors cursor-pointer {snippetTab === 'curl'
                      ? 'bg-[var(--code-bg)] text-[var(--brand-primary)] font-semibold border border-[var(--code-border)]'
                      : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
                  >
                    curl.sh
                  </button>
                </div>
              </div>

              <button
                type="button"
                onclick={copyCode}
                class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-mono text-[var(--text-muted)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-surface-hover)] transition-colors cursor-pointer"
                title="Copy snippet"
              >
                {#if copied}
                  <Check class="w-3 h-3 text-[var(--brand-primary)]" />
                  <span class="text-[var(--brand-primary)]">Copied</span>
                {:else}
                  <Copy class="w-3 h-3" />
                  <span>Copy</span>
                {/if}
              </button>
            </div>

            <!-- Syntax Highlighted Code Body with Line Numbers (Adapts to Light & Dark Theme) -->
            <div class="p-3.5 text-[11px] font-mono leading-[1.65] overflow-x-auto select-text">
              {#if snippetTab === "sdk"}
                <div class="grid grid-cols-[18px_1fr] gap-x-3">
                  <span class="text-[var(--code-lineno)] select-none text-right">1</span>
                  <div><span class="text-[var(--code-keyword)]">import</span> <span class="text-[var(--code-plain)]">&#123;</span> <span class="text-[var(--code-entity)]">Kinetiq</span> <span class="text-[var(--code-plain)]">&#125;</span> <span class="text-[var(--code-keyword)]">from</span> <span class="text-[var(--code-string)]">'@kinetiq/sdk'</span><span class="text-[var(--code-punct)]">;</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">2</span>
                  <div></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">3</span>
                  <div><span class="text-[var(--code-keyword)]">const</span> <span class="text-[var(--code-prop)]">kinetiq</span> <span class="text-[var(--code-keyword)]">=</span> <span class="text-[var(--code-keyword)]">new</span> <span class="text-[var(--code-entity)]">Kinetiq</span><span class="text-[var(--code-plain)]">(&#123;</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">4</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">apiKey</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-plain)]">process.env.</span><span class="text-[var(--code-accent-str)]">KINETIQ_API_KEY</span><span class="text-[var(--code-punct)]">,</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">5</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">endpoint</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-string)]">'https://ingest.kinetiq.io'</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">6</span>
                  <div><span class="text-[var(--code-plain)]">&#125;);</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">7</span>
                  <div></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">8</span>
                  <div><span class="text-[var(--code-keyword)]">await</span> <span class="text-[var(--code-prop)]">kinetiq</span><span class="text-[var(--code-punct)]">.</span><span class="text-[var(--code-fn)]">track</span><span class="text-[var(--code-plain)]">(</span><span class="text-[var(--code-accent-str)]">'checkout.completed'</span><span class="text-[var(--code-punct)]">,</span> <span class="text-[var(--code-plain)]">&#123;</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">9</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">plan</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-string)]">'pro_annual'</span><span class="text-[var(--code-punct)]">,</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">10</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">currency</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-string)]">'IDR'</span><span class="text-[var(--code-punct)]">,</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">11</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">value</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-entity)]">1490000</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">12</span>
                  <div><span class="text-[var(--code-plain)]">&#125;);</span></div>
                </div>
              {:else}
                <div class="grid grid-cols-[18px_1fr] gap-x-3">
                  <span class="text-[var(--code-lineno)] select-none text-right">1</span>
                  <div><span class="text-[var(--code-accent-str)]">curl</span> <span class="text-[var(--code-prop)]">-X</span> <span class="text-[var(--code-entity)]">POST</span> <span class="text-[var(--code-string)]">http://localhost:8080/v1/events</span> <span class="text-[var(--code-punct)]">\</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">2</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">-H</span> <span class="text-[var(--code-string)]">"Content-Type: application/json"</span> <span class="text-[var(--code-punct)]">\</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">3</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">-H</span> <span class="text-[var(--code-accent-str)]">"{exampleKeyHeader}"</span> <span class="text-[var(--code-punct)]">\</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">4</span>
                  <div class="pl-3"><span class="text-[var(--code-prop)]">-d</span> <span class="text-[var(--code-plain)]">'&#123;</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">5</span>
                  <div class="pl-6"><span class="text-[var(--code-prop)]">"event_name"</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-accent-str)]">"checkout.completed"</span><span class="text-[var(--code-punct)]">,</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">6</span>
                  <div class="pl-6"><span class="text-[var(--code-prop)]">"user_id"</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-string)]">"usr_01J9X2K8M"</span><span class="text-[var(--code-punct)]">,</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">7</span>
                  <div class="pl-6"><span class="text-[var(--code-prop)]">"properties"</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-plain)]">&#123;</span> <span class="text-[var(--code-prop)]">"plan"</span><span class="text-[var(--code-punct)]">:</span> <span class="text-[var(--code-string)]">"pro_annual"</span> <span class="text-[var(--code-plain)]">&#125;</span></div>

                  <span class="text-[var(--code-lineno)] select-none text-right">8</span>
                  <div class="pl-3"><span class="text-[var(--code-plain)]">&#125;'</span></div>
                </div>
              {/if}
            </div>

            <!-- Bottom Status Bar -->
            <div class="px-3.5 py-2 border-t border-[var(--code-border)] bg-[var(--code-header-bg)] flex items-center justify-between text-[10px] font-mono">
              <span class="text-[var(--brand-primary)] font-semibold flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-[var(--brand-primary)]"></span>
                Event Recorded
              </span>
              <span class="text-[var(--text-muted)]">Updated on your dashboard</span>
            </div>
          </div>

          <!-- Friendly Feature Highlights -->
          <div class="space-y-2 pt-1">
            <div class="text-[11px] font-mono uppercase tracking-wider text-[var(--text-muted)]">
              Why Teams Use Kinetiq
            </div>

            <div class="grid grid-cols-1 gap-2">
              <div class="p-3 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] flex items-center justify-between">
                <div>
                  <div class="text-xs font-semibold text-[var(--text-primary)]">Instant Activity Tracking</div>
                  <div class="text-[11px] text-[var(--text-muted)]">See signups, clicks, and purchases as they happen</div>
                </div>
                <span class="text-[11px] font-mono font-semibold text-[var(--brand-primary)]">Real-Time</span>
              </div>

              <div class="p-3 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] flex items-center justify-between">
                <div>
                  <div class="text-xs font-semibold text-[var(--text-primary)]">Simple Team Collaboration</div>
                  <div class="text-[11px] text-[var(--text-muted)]">Invite teammates and manage access per project</div>
                </div>
                <span class="text-[11px] font-mono text-[var(--text-secondary)]">Multi-Project</span>
              </div>

              <div class="p-3 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] flex items-center justify-between">
                <div>
                  <div class="text-xs font-semibold text-[var(--text-primary)]">Privacy-First & Self-Hostable</div>
                  <div class="text-[11px] text-[var(--text-muted)]">Run on Kinetiq Cloud or host on your own server</div>
                </div>
                <span class="text-[11px] font-mono text-[var(--text-secondary)]">Open Source</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Bottom Metadata -->
        <div class="pt-5 border-t border-[var(--border-subtle)] flex items-center justify-between text-[11px] font-mono text-[var(--text-muted)]">
          <span>2-Step Verification Ready</span>
          <span>Cloud & Self-Hosted</span>
        </div>
      </div>
    </div>
  </main>

  <!-- Minimal Footer -->
  <footer class="w-full max-w-[1120px] mx-auto px-6 pb-6 flex items-center justify-between text-[11px] text-[var(--text-muted)]">
    <span>Kinetiq Analytics</span>
    <span class="font-mono">Simple, Real-Time Product Insights</span>
  </footer>
</div>
