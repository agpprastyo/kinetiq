export type FunnelOrder = 'ordered' | 'strict';

export interface FunnelOptions {
  window?: string;
  order?: FunnelOrder;
}

export const FUNNEL_WINDOWS = ['1h', '1d', '7d', '14d', '30d'];

export function funnelOptionsToQuery(o?: FunnelOptions): string {
  if (!o) return '';
  let q = '';
  if (o.window) q += `&window=${encodeURIComponent(o.window)}`;
  if (o.order === 'strict') q += '&order=strict';
  return q;
}

export function dropoffPage(offset: number, limit: number, total: number) {
  return {
    from: total === 0 ? 0 : offset + 1,
    to: Math.min(offset + limit, total),
    hasPrev: offset > 0,
    hasNext: offset + limit < total
  };
}
