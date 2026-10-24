# Selected local monitoring configuration

These existing configuration files are explicitly selected from PR31 to satisfy the candidate Compose mounts. The [canonical reconciliation](../docs/CANONICAL_RECONCILIATION.md) controls acceptance and limitations. No live monitoring service, credential or dashboard was changed.

Configuration syntax and mount existence are separate from actual metrics/dashboard acceptance. Keep all published ports on loopback and provide an explicit local Grafana password. Do not treat this historical monitoring configuration as production deployment or security approval.
