import { defineRailway, project, service, preserve } from "railway/iac";

// Own only this fresh staging backend; do not create/alter database, bucket,
// domains, original installations or any other environment in this partial.
export const partial = "sfd-staging-backend";

export default defineRailway((ctx) => {
  if (ctx.projectName !== "secure-file-drop-staging" || ctx.environment !== "staging") {
    throw new Error("STOP: only secure-file-drop-staging / staging is permitted");
  }
  const backend = service("backend", {
    build: { builder: "DOCKERFILE", dockerfilePath: "Dockerfile.backend" },
    healthcheck: "/ready",
    healthcheckTimeout: 300,
    replicas: 1,
    deploy: { drainingSeconds: 25 },
    env: {
      RAILWAY_DOCKERFILE_PATH: "Dockerfile.backend",
      PORT: "8080",
      SFD_ADDR: ":8080",
      SFD_MAX_UPLOAD_BYTES: "10485760",
      SFD_MAX_REQUEST_BYTES: "11534336",
      SFD_S3_BUCKET_LOOKUP: "virtual",
      SFD_EMAIL_PROVIDER: "resend",
      SFD_CLEANUP_ENABLED: "false",
      DATABASE_URL: preserve(),
      SFD_ENV: preserve(),
      SFD_S3_PRIVACY_MODE: preserve(),
      SFD_S3_ENDPOINT: preserve(),
      SFD_S3_ACCESS_KEY: preserve(),
      SFD_S3_SECRET_KEY: preserve(),
      SFD_BUCKET: preserve(),
      SFD_ADMIN_USER: preserve(),
      SFD_ADMIN_PASS: preserve(),
      SFD_SESSION_SECRET: preserve(),
      SFD_DOWNLOAD_SECRET: preserve(),
      SFD_PUBLIC_BASE_URL: preserve(),
      SFD_EMAIL_FROM: preserve(),
      SFD_RESEND_API_KEY: preserve(),
    },
  });
  return project("secure-file-drop-staging", { resources: [backend] });
});
