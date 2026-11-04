const {test} = require('node:test');
const assert = require('node:assert/strict');
const {createBrowserTrace} = require('../auth/browser-diagnostics.cjs');
const origin = 'https://localhost:32123';
test('browser receipt records fixed API status without URL credentials, queries or fragments', () => {
  const trace = createBrowserTrace(origin);
  trace.record(origin.replace('https://', 'https://private-user:private-password@') + '/me?token=private-token#private-fragment', 401);
  assert.deepEqual(trace.snapshot(), [{path:'/me', status:401}]);
  assert.ok(!JSON.stringify(trace.snapshot()).includes('private-'));
});
test('browser receipt rejects foreign origins, signed-link paths, invalid URLs and status values', () => {
  const trace = createBrowserTrace(origin);
  for (const url of ['https://foreign.invalid/me', origin + '/download?token=private-token', origin + '/receive?invitation=private-id', origin + '/user/files/private-id', 'not a URL']) trace.record(url, 200);
  for (const status of [99,600,'200',NaN]) trace.record(origin + '/quota', status);
  assert.deepEqual(trace.snapshot(), []);
});
test('browser receipt is bounded and callers cannot mutate retained events', () => {
  const trace = createBrowserTrace(origin);
  for (let i=0; i<40; i++) trace.record(origin + '/quota', 200+i);
  const snapshot = trace.snapshot();
  assert.equal(snapshot.length,32);assert.equal(snapshot[0].status,208);
  snapshot[0].path='private-value';snapshot.push({path:'private-value', status:200});
  assert.equal(trace.snapshot().length,32);assert.equal(trace.snapshot()[0].path,'/quota');
});
