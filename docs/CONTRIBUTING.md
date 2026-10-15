# Contributing

Use the [root contributor guide](../CONTRIBUTING.md), [agent instructions](../AGENTS.md) and [GHP-01](GHP-01.md). Go 1.26.9 is the pinned toolchain. GCC, OpenSSL headers, Make and Python 3 support native verification.

Run go test ./... -v, go vet ./..., relevant race checks and make -C native test. Unit runs explicitly skip the Docker package without SFD_E2E=1; do not report that skip as acceptance. The opt-in full-stack test requires a local Docker engine and verifies resource ownership before cleanup. See [README](../README.md) for commands.

Never use existing databases, buckets or volumes for tests. Keep workflow documentation installation separate from application commits, preserve dated history and roadmap ideas, and publish one complete exact-revision PR handover. No master integration is implied.
