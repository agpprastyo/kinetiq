<script lang="ts">
  import Card from "$lib/components/ui/Card.svelte";
  import Button from "$lib/components/ui/Button.svelte";
  import { api, type CatalogProperty, type CatalogValue } from "$lib/api";
  import { buildFilterParam, opsForType, type FilterCondition, type QueryExtras } from "$lib/filters";
  import { Filter, Plus, X } from "lucide-svelte";

  interface Props {
    projectId: string;
    onApply: (extras: QueryExtras) => void;
  }

  let { projectId, onApply }: Props = $props();

  let properties = $state<CatalogProperty[]>([]);
  let values = $state<Record<number, CatalogValue[]>>({});
  let conditions = $state<FilterCondition[]>([]);
  let group = $state<"and" | "or">("and");
  let breakdownBy = $state("");

  const opLabels: Record<string, string> = {
    eq: "equals", neq: "not equals", contains: "contains", gt: "greater than",
    lt: "less than", in: "in (comma list)", is_set: "is set", is_not_set: "is not set",
  };

  $effect(() => {
    const id = projectId;
    if (!id) return;
    Promise.all([
      api.listCatalogProperties(id, "event").catch(() => []),
      api.listCatalogProperties(id, "person").catch(() => []),
    ]).then(([ev, person]) => (properties = [...ev, ...person]));
  });

  function propFor(c: FilterCondition) {
    return properties.find((p) => p.name === c.property && p.scope === (c.scope ?? "event"));
  }

  async function onPropertyChange(i: number) {
    const c = conditions[i];
    const match = properties.find((p) => p.name === c.property);
    if (match) c.scope = match.scope;
    const ops = opsForType(propFor(c)?.type ?? "unknown");
    if (!ops.includes(c.op)) c.op = ops[0];
    values[i] = c.property ? await api.listCatalogValues(projectId, c.property, c.scope ?? "event").catch(() => []) : [];
  }

  function addCondition() {
    conditions.push({ property: "", op: "eq", value: "", scope: "event" });
  }

  function removeCondition(i: number) {
    conditions.splice(i, 1);
    delete values[i];
  }

  function apply(e: SubmitEvent) {
    e.preventDefault();
    onApply({ filter: buildFilterParam(conditions, group), breakdownBy: breakdownBy.trim() || undefined });
  }

  function clear() {
    conditions = [];
    breakdownBy = "";
    onApply({});
  }
</script>

<Card class="space-y-3">
  <div class="flex items-center gap-2.5">
    <div class="w-8 h-8 rounded-full bg-[var(--bg-surface-elevated)] border border-[var(--border-subtle)] flex items-center justify-center text-[var(--primary-emerald)]">
      <Filter class="w-4 h-4" />
    </div>
    <div>
      <h2 class="text-sm font-semibold text-[var(--text-primary)]">Filters &amp; Breakdown</h2>
      <p class="text-[11px] text-[var(--text-muted)]">Applied to summary, top events, volume, funnel, breakdown and retention</p>
    </div>
  </div>

  <datalist id="filter-properties">
    {#each properties as p}
      <option value={p.name}>{p.scope} · {p.type}</option>
    {/each}
  </datalist>

  <form onsubmit={apply} class="space-y-2.5">
    {#if conditions.length > 1}
      <div class="flex items-center gap-2 text-[11px] text-[var(--text-muted)]">
        <label for="filter-group">Match</label>
        <select id="filter-group" bind:value={group} class="px-2 py-1 rounded-lg bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs">
          <option value="and">all conditions (AND)</option>
          <option value="or">any condition (OR)</option>
        </select>
      </div>
    {/if}

    {#each conditions as c, i}
      <div class="flex flex-wrap items-center gap-2">
        <input
          aria-label="Property"
          list="filter-properties"
          bind:value={c.property}
          onchange={() => onPropertyChange(i)}
          placeholder="property"
          class="flex-1 min-w-32 px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs"
        />
        <select aria-label="Operator" bind:value={c.op} class="px-2 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs">
          {#each opsForType(propFor(c)?.type ?? "unknown") as op}
            <option value={op}>{opLabels[op]}</option>
          {/each}
        </select>
        {#if c.op !== "is_set" && c.op !== "is_not_set"}
          <input
            aria-label="Value"
            list={`filter-values-${i}`}
            bind:value={c.value}
            placeholder="value"
            class="flex-1 min-w-24 px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs"
          />
          <datalist id={`filter-values-${i}`}>
            {#each values[i] ?? [] as v}
              <option value={v.value}></option>
            {/each}
          </datalist>
        {/if}
        <button type="button" aria-label="Remove condition" onclick={() => removeCondition(i)} class="p-1.5 rounded-lg text-[var(--text-muted)] hover:text-[var(--text-primary)]">
          <X class="w-3.5 h-3.5" />
        </button>
      </div>
    {/each}

    <div class="flex flex-wrap items-center gap-2">
      <Button type="button" variant="secondary" size="sm" onclick={addCondition}>
        <Plus class="w-3.5 h-3.5" /> Add filter
      </Button>
      <input
        aria-label="Breakdown by"
        list="filter-properties"
        bind:value={breakdownBy}
        placeholder="breakdown by (optional)"
        class="flex-1 min-w-32 px-3 py-1.5 rounded-xl bg-[var(--bg-surface)] border border-[var(--border-subtle)] text-xs"
      />
      <Button type="submit" variant="primary" size="sm">Apply</Button>
      <Button type="button" variant="ghost" size="sm" onclick={clear}>Clear</Button>
    </div>
  </form>
</Card>
