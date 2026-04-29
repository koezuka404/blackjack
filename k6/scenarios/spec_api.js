import http from 'k6/http';
import { check, sleep } from 'k6';

const apiBaseRaw = __ENV.K6_API_BASE_URL || 'http://127.0.0.1:8080/api';
const apiBase = apiBaseRaw.replace(/\/+$/, '');
const rootBase = apiBase.endsWith('/api') ? apiBase.slice(0, -4) : apiBase;

export const options = {
  vus: Number(__ENV.K6_VUS || 100),
  duration: __ENV.K6_DURATION || '5m',
  thresholds: {
    http_req_failed: ['rate<0.1'],
    http_req_duration: ['p(95)<1000'],
  },
};

export default function () {
  const healthRes = http.get(`${rootBase}/health`);
  check(healthRes, {
    'health status is 200': (r) => r.status === 200,
  });

  const username = `k6_${__VU}_${__ITER}_${Date.now()}`;
  const password = 'password12';
  const signupRes = http.post(
    `${apiBase}/auth/signup`,
    JSON.stringify({ username, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(signupRes, {
    'signup status is 200 or 201': (r) => r.status === 200 || r.status === 201,
  });

  sleep(1);
}
