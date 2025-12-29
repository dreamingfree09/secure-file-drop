import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');
const uploadDuration = new Trend('upload_duration');
const downloadDuration = new Trend('download_duration');

// Test configuration
export const options = {
  stages: [
    { duration: '2m', target: 10 },  // Ramp up to 10 users
    { duration: '5m', target: 50 },  // Ramp up to 50 users
    { duration: '5m', target: 50 },  // Stay at 50 users
    { duration: '2m', target: 100 }, // Spike to 100 users
    { duration: '5m', target: 100 }, // Stay at 100 users
    { duration: '5m', target: 0 },   // Ramp down
  ],
  thresholds: {
    'http_req_duration': ['p(95)<500', 'p(99)<1000'], // 95% < 500ms, 99% < 1s
    'http_req_failed': ['rate<0.01'],  // Error rate < 1%
    'errors': ['rate<0.05'],           // Custom error rate < 5%
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TEST_USERNAME = `testuser_${Date.now()}`;
const TEST_PASSWORD = 'TestPassword123';

export function setup() {
  // Register test user
  const registerRes = http.post(`${BASE_URL}/register`, JSON.stringify({
    email: `${TEST_USERNAME}@example.com`,
    username: TEST_USERNAME,
    password: TEST_PASSWORD,
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  if (registerRes.status !== 201) {
    console.log('Failed to register user:', registerRes.body);
  }

  return { username: TEST_USERNAME, password: TEST_PASSWORD };
}

export default function (data) {
  // Login
  const loginRes = http.post(`${BASE_URL}/login`, JSON.stringify({
    username: data.username,
    password: data.password,
  }), {
    headers: { 'Content-Type': 'application/json' },
  });

  const loginSuccess = check(loginRes, {
    'login successful': (r) => r.status === 200,
  });

  if (!loginSuccess) {
    errorRate.add(1);
    return;
  }

  const sessionCookie = loginRes.cookies.sfd_session[0].value;

  // Get quota
  const quotaRes = http.get(`${BASE_URL}/quota`, {
    cookies: { sfd_session: sessionCookie },
  });

  check(quotaRes, {
    'quota retrieved': (r) => r.status === 200,
  });

  // Create file metadata
  const fileName = `test-${Date.now()}.txt`;
  const fileContent = 'Hello World from k6 load test!';
  
  const metadataRes = http.post(`${BASE_URL}/files`, JSON.stringify({
    orig_name: fileName,
    content_type: 'text/plain',
    size_bytes: fileContent.length,
  }), {
    headers: { 'Content-Type': 'application/json' },
    cookies: { sfd_session: sessionCookie },
  });

  const metadataSuccess = check(metadataRes, {
    'metadata created': (r) => r.status === 201,
  });

  if (!metadataSuccess) {
    errorRate.add(1);
    return;
  }

  const fileId = JSON.parse(metadataRes.body).id;

  // Upload file
  const uploadStart = Date.now();
  const uploadData = {
    file: http.file(fileContent, fileName, 'text/plain'),
  };

  const uploadRes = http.post(`${BASE_URL}/upload?id=${fileId}`, uploadData, {
    cookies: { sfd_session: sessionCookie },
  });

  uploadDuration.add(Date.now() - uploadStart);

  const uploadSuccess = check(uploadRes, {
    'upload successful': (r) => r.status === 200,
  });

  if (!uploadSuccess) {
    errorRate.add(1);
    return;
  }

  // Wait for hashing
  sleep(2);

  // Create download link
  const linkRes = http.post(`${BASE_URL}/links`, JSON.stringify({
    file_id: fileId,
    expires_in_hours: 1,
  }), {
    headers: { 'Content-Type': 'application/json' },
    cookies: { sfd_session: sessionCookie },
  });

  const linkSuccess = check(linkRes, {
    'link created': (r) => r.status === 201,
  });

  if (!linkSuccess) {
    errorRate.add(1);
    return;
  }

  const downloadUrl = JSON.parse(linkRes.body).url;

  // Download file
  const downloadStart = Date.now();
  const downloadRes = http.get(downloadUrl);

  downloadDuration.add(Date.now() - downloadStart);

  check(downloadRes, {
    'download successful': (r) => r.status === 200,
    'content correct': (r) => r.body === fileContent,
  });

  // List user files
  const filesRes = http.get(`${BASE_URL}/user/files`, {
    cookies: { sfd_session: sessionCookie },
  });

  check(filesRes, {
    'files listed': (r) => r.status === 200,
  });

  // Delete file
  const deleteRes = http.del(`${BASE_URL}/user/files/${fileId}`, null, {
    cookies: { sfd_session: sessionCookie },
  });

  check(deleteRes, {
    'file deleted': (r) => r.status === 204,
  });

  // Logout
  http.post(`${BASE_URL}/logout`, null, {
    cookies: { sfd_session: sessionCookie },
  });

  sleep(1);
}

export function teardown(data) {
  console.log('Test completed');
}
