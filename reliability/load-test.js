import http from 'k6/http';
import { check, sleep } from 'k6';
export const options = {
  stages: [{ duration: '30s', target: 5 }, { duration: '1m', target: 20 }, { duration: '30s', target: 0 }],
  thresholds: { http_req_failed: ['rate<0.001'], http_req_duration: ['p(95)<500'] },
};
export default function () {
  const response = http.get(`${__ENV.TARGET_URL}/api/hello`);
  check(response, { 'HTTP 200': r => r.status === 200 });
  sleep(1);
}
