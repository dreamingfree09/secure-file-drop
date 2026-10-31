# CA transition design fixtures

This directory is a non-executing design package. Start with [the technical design](../../docs/LOCAL_PKI_TRANSITION_DESIGN.md).

- policy.json records proposed parameters; all future real identities are null.
- dependency-inventory.json records dated public/source/metadata observations and clearly labelled historical disk receipts.
- model.py evaluates fabricated policy scenarios only. It cannot authorise or execute real operations.
- test_model.py exercises phase boundaries, failures, retirement and expiry rules without cryptographic material, sockets, processes or trust APIs.
- synthetic-results.json contains actual test outcomes and source hashes; it is not TLS, signature or application evidence.
- source-manifest.json binds this package's files and the authority decision (excluding itself).

Reproduce only the inert model tests from this directory: python3 -I -B test_model.py. Standard-library unittest loads model.py explicitly; no package install or setup hook. The runner writes synthetic-results.json in its own directory. Do not invoke original signer, checker, controller or Docker.

No key/certificate/CRL fixture is generated, no network request is sent, and no credentials are needed. Future real creation, Windows trust, publication, service selection and retirement remain separate Owner/Aether gates.
