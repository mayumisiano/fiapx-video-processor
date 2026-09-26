// Proves the functional requirements "process more than one video at a
// time" and "don't lose a request under load spikes" (docs/adr/0012),
// rather than trusting them by construction. See README.md for usage.

import http from 'k6/http';
import { check, sleep } from 'k6';

const IDENTITY_URL = __ENV.IDENTITY_API_URL || 'http://localhost:8081/api/v1';
const VIDEO_URL = __ENV.VIDEO_API_URL || 'http://localhost:8080/api/v1';
const VUS = parseInt(__ENV.VUS || '20', 10);
const COMPLETION_TIMEOUT_SECONDS = parseInt(__ENV.COMPLETION_TIMEOUT_SECONDS || '120', 10);

const VIDEO_FILE = open('./fixtures/sample.mp4', 'b');

export const options = {
  scenarios: {
    concurrent_uploads: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: 1,
      maxDuration: '2m',
    },
  },
  thresholds: {
    // The functional requirement under test: not a single upload lost
    // under a concurrent burst.
    'checks{check:upload_accepted}': ['rate==1.0'],
  },
  // teardown() polls for completion for up to COMPLETION_TIMEOUT_SECONDS
  // (default 120s) — k6's own teardownTimeout defaults to 60s, which
  // would cut that polling short, so it's raised to match with margin.
  teardownTimeout: `${COMPLETION_TIMEOUT_SECONDS + 30}s`,
};

export function setup() {
  const email = `loadtest-${Date.now()}@example.com`;
  const password = 'loadtest12345';

  const registerRes = http.post(
    `${IDENTITY_URL}/auth/register`,
    JSON.stringify({ name: 'Load Test', email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );

  if (registerRes.status !== 201) {
    throw new Error(`setup: register failed with status ${registerRes.status}: ${registerRes.body}`);
  }

  return { token: registerRes.json('token'), expectedUploads: VUS };
}

export default function (data) {
  const res = http.post(
    `${VIDEO_URL}/videos`,
    { videos: http.file(VIDEO_FILE, `stress-${__VU}-${__ITER}.mp4`, 'video/mp4') },
    { headers: { Authorization: `Bearer ${data.token}` } },
  );

  check(
    res,
    {
      upload_accepted: (r) => {
        if (r.status !== 201) {
          return false;
        }
        const results = r.json('results');
        return Array.isArray(results) && results.length === 1 && results[0].accepted === true;
      },
    },
    { check: 'upload_accepted' },
  );
}

// Accepted isn't enough proof on its own — this polls until every
// upload is actually processed.
export function teardown(data) {
  const deadline = Date.now() + COMPLETION_TIMEOUT_SECONDS * 1000;
  let requests = [];

  while (Date.now() < deadline) {
    const res = http.get(`${VIDEO_URL}/videos`, {
      headers: { Authorization: `Bearer ${data.token}` },
    });
    requests = res.json('requests') || [];

    const stillRunning = requests.filter((r) => r.status === 'PENDING' || r.status === 'PROCESSING');
    if (requests.length >= data.expectedUploads && stillRunning.length === 0) {
      break;
    }
    sleep(2);
  }

  const completed = requests.filter((r) => r.status === 'COMPLETED').length;
  const failed = requests.filter((r) => r.status === 'FAILED').length;
  const stillRunning = requests.filter((r) => r.status === 'PENDING' || r.status === 'PROCESSING').length;

  console.log(
    `teardown: ${requests.length}/${data.expectedUploads} requests observed, ` +
      `${completed} completed, ${failed} failed, ${stillRunning} still running at deadline`,
  );

  check(null, {
    'all uploaded videos reached a terminal state': () => stillRunning === 0 && requests.length >= data.expectedUploads,
    'no video failed processing': () => failed === 0,
  });
}
