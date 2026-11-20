import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PROJECT_ID = __ENV.PROJECT_ID || '';
const COOKIE = __ENV.COOKIE || 'user1_session_seed_k6';
const MODE = __ENV.MODE || 'warm'; // 'warm' or 'cold'

if (!PROJECT_ID) {
  console.warn('PROJECT_ID environment variable is not set. Requests may fail with 404.');
}

const analyticsLatency = new Trend('analytics_latency_ms', true);
const errorRate = new Rate('analytics_error_rate');
const queriesSuccess = new Counter('analytics_queries_success');

export const options = {
  scenarios: {
    analytics_queries: {
      executor: 'constant-vus',
      vus: parseInt(__ENV.VUS || '10', 10),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.01'],
    'http_req_duration': ['p(95)<300'],
  },
};

export default function () {
  let fromDate;
  let toDate;

  if (MODE === 'cold') {
    // Generate varying randomized past time windows to intentionally miss Redis cache
    const randomHoursAgo = Math.floor(Math.random() * 500) + 48;
    const fromTime = new Date(Date.now() - (randomHoursAgo + 24) * 3600 * 1000);
    const toTime = new Date(Date.now() - randomHoursAgo * 3600 * 1000);
    fromDate = fromTime.toISOString();
    toDate = toTime.toISOString();
  } else {
    // Warm mode: fixed range aligned with cached queries
    const now = new Date();
    const yesterday = new Date(now.getTime() - 24 * 3600 * 1000);
    fromDate = yesterday.toISOString();
    toDate = now.toISOString();
  }

  const params = {
    headers: {
      Cookie: COOKIE,
    },
  };

  // Test 1: Summary query
  const summaryUrl = `${BASE_URL}/api/v1/projects/${PROJECT_ID}/analytics/summary?from=${encodeURIComponent(fromDate)}&to=${encodeURIComponent(toDate)}`;
  const startSummary = Date.now();
  const summaryRes = http.get(summaryUrl, Object.assign({}, params, { tags: { name: 'GetSummary' } }));
  analyticsLatency.add(Date.now() - startSummary);

  const summaryOk = check(summaryRes, {
    'summary status is 200': (r) => r.status === 200,
  });

  if (summaryOk) {
    queriesSuccess.add(1);
    errorRate.add(0);
  } else {
    errorRate.add(1);
  }

  // Test 2: Volume query
  const volumeUrl = `${BASE_URL}/api/v1/projects/${PROJECT_ID}/analytics/volume?from=${encodeURIComponent(fromDate)}&to=${encodeURIComponent(toDate)}&interval=hour`;
  const startVolume = Date.now();
  const volumeRes = http.get(volumeUrl, Object.assign({}, params, { tags: { name: 'GetVolume' } }));
  analyticsLatency.add(Date.now() - startVolume);

  const volumeOk = check(volumeRes, {
    'volume status is 200': (r) => r.status === 200,
  });

  if (volumeOk) {
    queriesSuccess.add(1);
    errorRate.add(0);
  } else {
    errorRate.add(1);
  }

  sleep(0.05);
}
