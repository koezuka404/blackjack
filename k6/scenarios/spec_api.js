import http from 'k6/http';
import { check, sleep } from 'k6';

const apiBaseRaw = __ENV.K6_API_BASE_URL || 'http://127.0.0.1:8080/api';
const apiBase = apiBaseRaw.replace(/\/+$/, '');
const rootBase = apiBase.endsWith('/api') ? apiBase.slice(0, -4) : apiBase;
const runID = __ENV.K6_RUN_ID || `${Date.now()}`;
const password = __ENV.K6_USER_PASSWORD || 'password12';
const vus = Number(__ENV.K6_VUS || 100);

export const options = {
  vus,
  duration: __ENV.K6_DURATION || '5m',
  setupTimeout: __ENV.K6_SETUP_TIMEOUT || '5m',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<1500'],
    'http_req_failed{endpoint:me}': ['rate<0.05'],
    'http_req_duration{endpoint:me}': ['p(95)<1500'],
  },
};

export function setup() {
  const healthRes = http.get(`${rootBase}/health`, { tags: { endpoint: 'health' } });
  check(healthRes, {
    'health status is 200': (r) => r.status === 200,
  });

  const tokens = [];
  for (let i = 1; i <= vus; i += 1) {
    const username = `k6_${runID}_${i}`;
    const email = `${username}@k6.local`;
    const signupRes = http.post(
      `${apiBase}/auth/signup`,
      JSON.stringify({ username, email, password }),
      {
        headers: { 'Content-Type': 'application/json' },
        tags: { endpoint: 'signup' },
        responseType: 'text',
      },
    );
    if (signupRes.status !== 200 && signupRes.status !== 201 && signupRes.status !== 409) {
      throw new Error(`setup signup failed for ${username}: status=${signupRes.status} body=${signupRes.body}`);
    }

    const loginRes = http.post(
      `${apiBase}/auth/login`,
      JSON.stringify({ email, password }),
      {
        headers: { 'Content-Type': 'application/json' },
        tags: { endpoint: 'login' },
        responseType: 'text',
      },
    );
    if (loginRes.status !== 200) {
      throw new Error(`setup login failed for ${username}: status=${loginRes.status} body=${loginRes.body}`);
    }
    const token = loginRes.json('data.access_token');
    if (!token) {
      throw new Error(`setup login returned empty token for ${username}`);
    }
    tokens.push(token);
  }

  return { tokens };
}

export default function (data) {
  const token = data?.tokens?.[__VU - 1];
  if (!token) {
    throw new Error(`missing token for vu=${__VU}`);
  }
  const meRes = http.get(`${apiBase}/me`, {
    headers: { Authorization: `Bearer ${token}` },
    tags: { endpoint: 'me' },
  });
  check(meRes, {
    'me status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
