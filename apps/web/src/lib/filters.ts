export type FilterOp = 'eq' | 'neq' | 'contains' | 'gt' | 'lt' | 'in' | 'is_set' | 'is_not_set';
export type FilterScope = 'event' | 'person';

export interface FilterCondition {
  property: string;
  op: FilterOp;
  value: string;
  scope?: FilterScope;
}

const ALL_OPS: FilterOp[] = ['eq', 'neq', 'contains', 'gt', 'lt', 'in', 'is_set', 'is_not_set'];

export function opsForType(type: string): FilterOp[] {
  if (type === 'number') return ALL_OPS.filter((o) => o !== 'contains');
  if (type === 'boolean') return ['eq', 'neq', 'is_set', 'is_not_set'];
  if (type === 'string') return ALL_OPS.filter((o) => o !== 'gt' && o !== 'lt');
  return ALL_OPS;
}

function toLeaf(c: FilterCondition): Record<string, unknown> | null {
  const property = c.property.trim();
  if (!property) return null;
  const leaf: Record<string, unknown> = { property, op: c.op };
  if (c.scope === 'person') leaf.scope = 'person';
  if (c.op === 'is_set' || c.op === 'is_not_set') return leaf;
  if (c.op === 'gt' || c.op === 'lt') {
    const n = Number(c.value);
    if (c.value.trim() === '' || !Number.isFinite(n)) return null;
    leaf.value = n;
  } else if (c.op === 'in') {
    const items = c.value.split(',').map((s) => s.trim()).filter(Boolean);
    if (items.length === 0) return null;
    leaf.value = items;
  } else {
    if (c.value === '') return null;
    leaf.value = c.value;
  }
  return leaf;
}

/** Serialize UI conditions to the API filter DSL ('' when nothing is complete). */
export function buildFilterParam(conditions: FilterCondition[], group: 'and' | 'or'): string {
  const leaves = conditions.map(toLeaf).filter((l): l is Record<string, unknown> => l !== null);
  if (leaves.length === 0) return '';
  return JSON.stringify(leaves.length === 1 ? leaves[0] : { group, filters: leaves });
}

export interface QueryExtras {
  filter?: string;
  breakdownBy?: string;
  breakdownLimit?: number;
}

// Analytics buckets follow the viewer's wall clock (API defaults to UTC without it).
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone;

export function extrasToQuery(extra?: QueryExtras): string {
  let q = '';
  if (!extra) return TZ ? `&tz=${encodeURIComponent(TZ)}` : q;
  if (extra.filter) q += `&filter=${encodeURIComponent(extra.filter)}`;
  if (extra.breakdownBy) {
    q += `&breakdown_by=${encodeURIComponent(extra.breakdownBy)}`;
    if (extra.breakdownLimit) q += `&breakdown_limit=${extra.breakdownLimit}`;
  }
  if (TZ) q += `&tz=${encodeURIComponent(TZ)}`;
  return q;
}
