# Monitoring Setup for Secure File Drop

This directory contains configuration files for the monitoring stack (Prometheus + Grafana).

## Quick Start

The monitoring stack is automatically included in `docker-compose.yml`:

```bash
docker-compose up -d
```

## Services

### Prometheus
- **URL**: http://localhost:9090
- **Metrics endpoint**: http://localhost:8080/metrics/prometheus
- **Config**: `prometheus.yml`

### Grafana
- **URL**: http://localhost:3000
- **Default credentials**: admin / admin (change on first login)
- **Default password**: Set via `GRAFANA_ADMIN_PASSWORD` in `.env`

## Pre-configured Dashboards

### Secure File Drop - Overview
- **UID**: `sfd-overview`
- **Location**: Automatically provisioned from `grafana/dashboards/sfd-overview.json`

**Metrics included**:
- System uptime
- Request rate (requests/sec)
- Total uploads and downloads
- Storage usage
- HTTP requests by endpoint
- Request duration percentiles (p50, p95, p99)
- Authentication metrics (login attempts, failures, active sessions)
- Upload/download rates over time

## Available Metrics

The Secure File Drop backend exposes the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sfd_http_requests_total` | Counter | Total HTTP requests by method, path, and status |
| `sfd_http_request_duration_seconds` | Histogram | HTTP request duration distribution |
| `sfd_uploads_total` | Counter | Total successful file uploads |
| `sfd_downloads_total` | Counter | Total file downloads |
| `sfd_storage_bytes_total` | Gauge | Total storage used in bytes |
| `sfd_login_attempts_total` | Counter | Total login attempts |
| `sfd_login_failures_total` | Counter | Failed login attempts |
| `sfd_active_sessions` | Gauge | Number of active user sessions |
| `sfd_uptime_seconds` | Gauge | Application uptime in seconds |

## Customization

### Adding Custom Dashboards

1. Create a JSON dashboard file in `grafana/dashboards/`
2. Restart Grafana: `docker-compose restart grafana`
3. Dashboard will be auto-provisioned

### Adding Alert Rules

Edit `prometheus.yml` to add alerting rules:

```yaml
rule_files:
  - 'alerts/*.yml'
```

### Grafana Configuration

Grafana provisioning files are located in:
- **Datasources**: `grafana/provisioning/datasources/`
- **Dashboards**: `grafana/provisioning/dashboards/`

## Retention

- **Prometheus**: Default 15 days (modify with `--storage.tsdb.retention.time` in docker-compose.yml)
- **Grafana**: Persistent storage via Docker volume `sfd_grafana_data`

## Security Considerations

### Production Deployment

1. **Change default Grafana password**:
   ```bash
   # In .env file
   GRAFANA_ADMIN_PASSWORD=your-secure-password
   ```

2. **Restrict access** (add to docker-compose.yml):
   ```yaml
   grafana:
     networks:
       - monitoring
     # Remove public port mapping, use reverse proxy instead
   ```

3. **Enable authentication** on Prometheus (use reverse proxy with basic auth)

4. **Use HTTPS** for both services in production

## Troubleshooting

### Prometheus can't scrape backend metrics

Check that the backend is running and metrics endpoint is accessible:
```bash
curl http://localhost:8080/metrics/prometheus
```

### Grafana can't connect to Prometheus

Verify Prometheus is reachable from Grafana container:
```bash
docker exec sfd_grafana wget -O- http://prometheus:9090/api/v1/status/config
```

### No data in dashboards

1. Check Prometheus targets: http://localhost:9090/targets
2. Verify backend is exposing metrics
3. Ensure time range in Grafana includes recent data

## Resources

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [PromQL Cheatsheet](https://promlabs.com/promql-cheat-sheet/)
