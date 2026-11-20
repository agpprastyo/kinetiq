<script lang="ts">
  import { cn } from "$lib/utils";

  interface Props {
    value: any;
    class?: string;
  }

  let { value, class: className = "" }: Props = $props();

  function escapeHtml(str: string): string {
    return str
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function syntaxHighlight(jsonObj: any): string {
    if (jsonObj === undefined || jsonObj === null) {
      return '<span class="text-rose-400">null</span>';
    }

    let json: string;
    try {
      json = typeof jsonObj === "string" ? jsonObj : JSON.stringify(jsonObj, null, 2);
    } catch {
      return escapeHtml(String(jsonObj));
    }

    // Replace JSON tokens with styled spans
    const tokenRegex = /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\\-]?\d+)?)/g;

    return json.replace(tokenRegex, (match) => {
      let cls = "text-sky-400"; // number
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          // Object key
          cls = "text-indigo-400 font-semibold";
        } else {
          // String value
          cls = "text-emerald-400";
        }
      } else if (/true|false/.test(match)) {
        cls = "text-amber-400 font-bold";
      } else if (/null/.test(match)) {
        cls = "text-rose-400 italic";
      }
      return `<span class="${cls}">${escapeHtml(match)}</span>`;
    });
  }
</script>

<pre class={cn("font-mono text-xs leading-relaxed overflow-x-auto p-2 rounded-lg bg-[var(--bg-canvas)] border border-[var(--border-subtle)] selection:bg-indigo-500/20", className)}><code>{@html syntaxHighlight(value)}</code></pre>
