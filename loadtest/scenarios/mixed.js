import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PROJECT_ID = __ENV.PROJECT_ID || '';
const API_KEY = __ENV.API_KEY || '';
const COOKIE = __ENV.COOKIE || 'user1_session_seed_k6';

const ingestLatency = new Trend('mixed_ingest_latency_ms', true);
const analyticsLatency = new Trend('mixed_analytics_latency_ms', true);
const mixedErrorRate = new Rate('mixed_error_rate');
const mixedOps = new Counter('mixed_operations_total');

export const options = {
  scenarios: {
    // 80% Ingestion traffic
    ingestion_stream: {
      executor: 'constant-vus',
      vus: Math.max(2, Math.floor(parseInt(__ENV.VUS || '10', 10) * 0.8)),
      duration: __ENV.DURATION || '30s',
      exec: 'runIngestion',
    },
    // 20% Dashboard Analytics traffic
    analytics_stream: {
      executor: 'constant-vus',
      vus: Math.max(1, Math.floor(parseInt(__ENV.VUS || '10', 10) * 0.2)),
      duration: __ENV.DURATION || '30s',
      exec: 'runAnalytics',
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.02'],
    'mixed_ingest_latency_ms': ['p(95)<250'],
    'mixed_analytics_latency_ms': ['p(95)<350'],
  },
};

const EVENT_NAMES = ['pageview', 'signup_click', 'checkout', 'api_call'];

function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

export function runIngestion() {
  const payload = JSON.stringify({
    event_id: generateUUID(),
    event_name: EVENT_NAMES[Math.floor(Math.random() * EVENT_NAMES.length)],
    distinct_id: `user_${Math.floor(Math.random() * 500)}`,
    properties: {
      page: '/dashboard',
      action: 'view_report',
    },
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
    },
    tags: { name: 'MixedPostIngest' },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/ingest`, payload, params);
  ingestLatency.add(Date.now() - start);
  mixedOps.add(1);

  const ok = check(res, {
    'ingest 202': (r) => r.status === 202,
  });

  mixedErrorRate.add(ok ? 0 : 1);
  sleep(0.05);
}

export function runAnalytics() {
  const now = new Date();
  const yesterday = new Date(now.getTime() - 24 * 3600 * 1000);
  const fromDate = yesterday.toISOString();
  const toDate = now.toISOString();

  const params = {
    headers: {
      Cookie: COOKIE,
    },
    tags: { name: 'MixedGetSummary' },
  };

  const summaryUrl = `${BASE_URL}/api/v1/projects/${PROJECT_ID}/analytics/summary?from=${encodeURIComponent(fromDate)}&to=${encodeURIComponent(toDate)}`;
  const start = Date.now();
  const res = http.get(summaryUrl, params);
  analyticsLatency.add(Date.now() - start);
  mixedOps.add(1);

  const ok = check(res, {
    'analytics 200': (r) => r.status === 200,
  });

  mixedErrorRate.add(ok ? 0 : 1);
  sleep(0.1);
}
