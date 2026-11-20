import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY = __ENV.API_KEY || '';

if (!API_KEY) {
  console.warn('API_KEY environment variable is not set. Requests may fail with 401.');
}

const acceptedEvents = new Counter('events_accepted');
const failedEvents = new Counter('events_failed');
const ingestLatency = new Trend('ingest_latency_ms', true);
const errorRate = new Rate('ingest_error_rate');

export const options = {
  scenarios: {
    ingestion: {
      executor: 'constant-vus',
      vus: parseInt(__ENV.VUS || '10', 10),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    'http_req_failed': ['rate<0.01'], // < 1% errors
    'http_req_duration': ['p(95)<200'], // 95% of requests under 200ms
  },
};

const EVENT_NAMES = [
  'pageview',
  'button_clicked',
  'item_viewed',
  'added_to_cart',
  'checkout_started',
  'purchase_completed',
];

function generateUUID() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

export default function () {
  const eventId = generateUUID();
  const distinctId = `user_${Math.floor(Math.random() * 1000)}`;
  const eventName = EVENT_NAMES[Math.floor(Math.random() * EVENT_NAMES.length)];

  const payload = JSON.stringify({
    event_id: eventId,
    event_name: eventName,
    distinct_id: distinctId,
    properties: {
      page: '/pricing',
      referrer: 'https://google.com',
      browser: 'Chrome',
      os: 'Linux',
      version: '1.2.0',
    },
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
    },
    tags: { name: 'PostIngest' },
  };

  const start = Date.now();
  const res = http.post(`${BASE_URL}/api/v1/ingest`, payload, params);
  const duration = Date.now() - start;
  ingestLatency.add(duration);

  const isAccepted = res.status === 202;
  const ok = check(res, {
    'status is 202 Accepted': (r) => r.status === 202,
    'has accepted status': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body && body.data && body.data.status === 'accepted';
      } catch (e) {
        return false;
      }
    },
  });

  if (isAccepted) {
    acceptedEvents.add(1);
    errorRate.add(0);
  } else {
    failedEvents.add(1);
    errorRate.add(1);
  }

  sleep(0.05); // slight pacing
}
