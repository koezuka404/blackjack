import http from 'k6/http';
import { check, sleep } from 'k6';

const apiBaseRaw = __ENV.K6_API_BASE_URL || 'http://127.0.0.1:8080/api';
const apiBase = apiBaseRaw.replace(/\/+$/, '');
const rootBase = apiBase.endsWith('/api') ? apiBase.slice(0, -4) : apiBase;
const runID = __ENV.K6_RUN_ID || `${Date.now()}`;
const password = __ENV.K6_USER_PASSWORD || 'password12';
let vuToken = '';
let vuUsername = '';

export const options = {
  vus: Number(__ENV.K6_VUS || 100),
  duration: __ENV.K6_DURATION || '5m',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1500'],
    'http_req_failed{endpoint:signup}': ['rate<0.05'],
    'http_req_failed{endpoint:login}': ['rate<0.05'],
    'http_req_failed{endpoint:me}': ['rate<0.05'],
    'http_req_duration{endpoint:signup}': ['p(95)<2000'],
    'http_req_duration{endpoint:login}': ['p(95)<2000'],
    'http_req_duration{endpoint:me}': ['p(95)<1500'],
  },
};

export function setup() {
  const healthRes = http.get(`${rootBase}/health`, { tags: { endpoint: 'health' } });
  check(healthRes, {
    'health status is 200': (r) => r.status === 200,
  });
}

export default function () {
  if (!vuUsername) {
    vuUsername = `k6_${runID}_${__VU}`;
  }

  if (!vuToken) {
    const signupRes = http.post(
      `${apiBase}/auth/signup`,
      JSON.stringify({ username: vuUsername, password }),
      { headers: { 'Content-Type': 'application/json' }, tags: { endpoint: 'signup' } },
    );
    check(signupRes, {
      'signup status is 200/201/409': (r) => r.status === 200 || r.status === 201 || r.status === 409,
    });
  }

  const loginRes = http.post(
    `${apiBase}/auth/login`,
    JSON.stringify({ username: vuUsername, password }),
    { headers: { 'Content-Type': 'application/json' }, tags: { endpoint: 'login' } },
  );
  const loginOk = check(loginRes, {
    'login status is 200': (r) => r.status === 200,
    'login has token': (r) => {
      try {
        const payload = r.json();
        return Boolean(payload?.data?.access_token);
      } catch (_) {
        return false;
      }
    },
  });

  if (loginOk && !vuToken) {
    vuToken = loginRes.json('data.access_token');
  }

  const meRes = http.get(`${apiBase}/me`, {
    headers: { Authorization: `Bearer ${vuToken}` },
    tags: { endpoint: 'me' },
  });
  check(meRes, {
    'me status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
