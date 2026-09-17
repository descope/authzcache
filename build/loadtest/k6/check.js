// Stress test for the authzcache edge container running the `loadtest` build (fake backend).
//
// Three scenarios, picked with SCENARIO=ramp|fill|smoke:
//   ramp  — ramping-arrival-rate, step-and-hold, finds the max sustained RPS.
//   fill  — constant-arrival-rate, 100% new keys, grows the cache to FILL_ENTRIES.
//   smoke — constant-arrival-rate at SMOKE_RPS; correctness at 500, knee bisection above that.
//
// Run `fill` first to reach a cache size, then `ramp` against that warm cache. Ramping RPS and
// cache size at the same time gives two confounded variables and an uninterpretable curve.

import http from 'k6/http';
import exec from 'k6/execution';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8189';
const PROJECT_ID = __ENV.PROJECT_ID || 'P2loadtest0000000000000000000';
const SCENARIO = __ENV.SCENARIO || 'ramp';
// fill exists to grow the cache, so it is all-new unless explicitly overridden.
const NEW_RATIO = Number(__ENV.NEW_RATIO || (SCENARIO === 'fill' ? 1 : 0.5));
const RES_CARD = Number(__ENV.RESOURCE_CARDINALITY || 0);
const TGT_CARD = Number(__ENV.TARGET_CARDINALITY || 0);
const TUPLES = Number(__ENV.TUPLES_PER_REQ || 1);
const FILL_ENTRIES = Number(__ENV.FILL_ENTRIES || 1000000);
const FILL_RPS = Number(__ENV.FILL_RPS || 5000);
const VERIFY = (__ENV.VERIFY_BODIES || 'false') === 'true';
// Ids restart at 0 every k6 run, so a second run against a warm cache would re-request keys that
// are already cached and its "miss" tag would lie. Offset each run past the previous one's range.
const ID_OFFSET = Number(__ENV.ID_OFFSET || 0);
// Set ABORT_ON_FAIL=false to let a saturated run finish, so the full latency curve is readable.
const ABORT = (__ENV.ABORT_ON_FAIL || 'true') === 'true';

// A "hit" never targets an id inserted within the last SAFETY iterations, which may still be in flight.
const SAFETY = 5000;

// Cache key is resource:target:relation. Capping both cardinalities collapses distinct ids onto the
// same key, so the cache would stop growing and "new" requests would silently become hits.
if (RES_CARD > 0 && TGT_CARD > 0) {
  throw new Error('cap RESOURCE_CARDINALITY or TARGET_CARDINALITY, not both — keys would stop being unique');
}

const rampScenario = {
  executor: 'ramping-arrival-rate',
  timeUnit: '1s',
  startRate: 500,
  // Little's law: in-flight = RPS x ~15ms mean (0.5 x ~0.5ms hit + 0.5 x 30ms miss).
  // 40k RPS needs ~600; 3-5x headroom for the tail at saturation.
  preAllocatedVUs: 800,
  maxVUs: 4000,
  // Flat steps, not a smooth ramp: a continuous ramp smears the knee and makes percentiles
  // unattributable. Each hold is >= 2m so a p99 rests on hundreds of thousands of samples.
  stages: [
    { target: 2000, duration: '30s' }, { target: 2000, duration: '2m' },
    { target: 5000, duration: '30s' }, { target: 5000, duration: '2m' },
    { target: 10000, duration: '30s' }, { target: 10000, duration: '2m' },
    { target: 20000, duration: '30s' }, { target: 20000, duration: '2m' },
    { target: 40000, duration: '30s' }, { target: 40000, duration: '2m' },
  ],
};

const fillScenario = {
  executor: 'constant-arrival-rate',
  rate: FILL_RPS,
  timeUnit: '1s',
  duration: Math.ceil(FILL_ENTRIES / (FILL_RPS * TUPLES)) + 's',
  preAllocatedVUs: Math.ceil(FILL_RPS * 0.05) + 50,
  maxVUs: 4000,
};

// Correctness run at 500 RPS, and the fixed-rate probe used to bisect the knee at higher SMOKE_RPS.
const SMOKE_RPS = Number(__ENV.SMOKE_RPS || 500);
// preAllocatedVUs === maxVUs on purpose. k6 allocates VUs lazily and drops iterations while it
// does, and those drops are indistinguishable from server saturation — they sank two bisect
// attempts. A fixed pool sized well above need means no allocation ever happens mid-run, so
// dropped_iterations only fires when the pool is genuinely exhausted by a slow server.
const SMOKE_VUS = Number(__ENV.SMOKE_VUS || Math.ceil(SMOKE_RPS * 0.15) + 200);
const smokeScenario = {
  executor: 'constant-arrival-rate',
  rate: SMOKE_RPS,
  timeUnit: '1s',
  duration: __ENV.SMOKE_DURATION || '30s',
  preAllocatedVUs: SMOKE_VUS,
  maxVUs: SMOKE_VUS,
};

const scenarios = { ramp: rampScenario, fill: fillScenario, smoke: smokeScenario };

export const options = {
  scenarios: { [SCENARIO]: scenarios[SCENARIO] || rampScenario },
  // Parsing JSON in k6's JS VM is the top reason the load generator becomes the bottleneck.
  // Do one VERIFY_BODIES=true run to confirm correctness, then leave it off.
  discardResponseBodies: !VERIFY,
  thresholds: {
    http_req_failed: [{ threshold: 'rate<0.01', abortOnFail: ABORT }],
    // The capacity signal, but only trustworthy with a fixed VU pool — see SMOKE_VUS.
    dropped_iterations: [{ threshold: 'count<100', abortOnFail: ABORT }],
    // The edge's true serving cost. The miss path carries a synthetic 10-50ms floor, so a
    // blended p95 is pinned near 50ms no matter how the edge behaves and tells you nothing.
    'http_req_duration{path:hit}': ['p(95)<25', 'p(99)<50'],
    'http_req_duration{path:miss}': ['p(99)<200'],
    checks: ['rate>0.99'],
  },
  summaryTrendStats: ['min', 'avg', 'med', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)', 'max', 'count'],
};

const headers = {
  'Content-Type': 'application/json',
  Authorization: `Bearer ${PROJECT_ID}:loadtest-key`,
};

function resourceOf(id) {
  return 'doc-' + (RES_CARD > 0 ? id % RES_CARD : id);
}

function targetOf(id) {
  return 'user-' + (TGT_CARD > 0 ? id % TGT_CARD : id);
}

function tuple(id) {
  return {
    resource: resourceOf(id),
    resourceType: 'doc',
    relation: 'owner',
    target: targetOf(id),
    targetType: 'user',
  };
}

export default function () {
  // iterationInTest is globally unique across VUs, so the hit/miss split needs no shared counter.
  const n = exec.scenario.iterationInTest;
  const created = Math.floor(n * NEW_RATIO); // ids [0, created) have been requested already
  const isNew = created > Math.floor((n - 1) * NEW_RATIO);

  let baseID;
  if (isNew) {
    baseID = ID_OFFSET + created * TUPLES;
  } else {
    // Hits stay inside this run's own range, so they are known-cached even at ID_OFFSET 0.
    const ceiling = Math.max(0, created * TUPLES - SAFETY);
    baseID = ID_OFFSET + (ceiling > 0 ? Math.floor(Math.random() * ceiling) : 0);
  }

  const tuples = [];
  for (let i = 0; i < TUPLES; i++) {
    tuples.push(tuple(baseID + i));
  }

  const res = http.post(`${BASE_URL}/v1/mgmt/fga/check`, JSON.stringify({ tuples }), {
    headers,
    tags: { path: isNew ? 'miss' : 'hit' },
  });

  if (VERIFY) {
    check(res, {
      'status 200': (r) => r.status === 200,
      'allowed and direct': (r) => {
        const t = r.json('tuples');
        return t && t.length === TUPLES && t.every((x) => x.allowed === true && x.info.direct === true);
      },
    });
  } else {
    check(res, { 'status 200': (r) => r.status === 200 });
  }
}
