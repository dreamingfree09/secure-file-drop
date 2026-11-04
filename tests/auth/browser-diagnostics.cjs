// Only fixed-path response statuses enter synthetic browser failure receipts.
const paths = new Set(['/', '/login', '/me', '/quota', '/user/files', '/files', '/metrics', '/logout']);
function createBrowserTrace(origin) {
  const events = [];
  return {
    record(rawURL, status) {
      try {
        const url = new URL(rawURL);
        if (url.origin !== origin || !paths.has(url.pathname) || !Number.isInteger(status) || status < 100 || status > 599) return;
        events.push({path: url.pathname, status});
        if (events.length > 32) events.shift();
      } catch (_) { /* Invalid URL values are never diagnostic output. */ }
    },
    snapshot() { return events.map(event => ({...event})); },
  };
}
module.exports = {createBrowserTrace};
