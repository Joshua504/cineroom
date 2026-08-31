Cineroom — Security & Implementation Plan

Overview

This document lists missing items required for the project to run smoothly and securely in production, plus a clear implementation plan for each item: rationale, concrete steps, files/PR touches, verification steps, and estimated effort.

Priority key
- P0: High priority / blocking for secure production
- P1: Important but not blocking
- P2: Nice-to-have / incremental

1) Continuous integration, tests, and dependency automation (P0)
Why: Ensure changes build, tests pass, and dependencies are scanned for vulnerabilities.
Implementation steps:
- Add GitHub Actions workflow (.github/workflows/ci.yml) that runs: go test ./..., go vet, golangci-lint (if added), and `go mod download`.
- Add Dependabot config for Go modules and docker images (.github/dependabot.yml).
- Add a lightweight `make ci` or GitHub action matrix for Go versions.
Files/PR touches:
- .github/workflows/ci.yml (new)
- .github/dependabot.yml (new)
- optionally Makefile (new) with `make test` and `make lint` targets
Verification:
- PR shows green CI on push; tests and linters pass.
Estimated effort: 1-2 days.

2) Secret management and environment guidance (P0)
Why: APP_SECRET, OAuth and SMTP credentials must be managed securely and rotated.
Implementation steps:
- Document secret requirements in README and add a checklist for production vault (HashiCorp Vault / AWS Secrets Manager / GCP Secret Manager).
- Add .env.example already present; add SECURITY.md guidance to never commit .env and how to inject secrets (env, systemd, docker secrets).
- Add health check to fail fast if critical secrets are missing.
Files/PR touches:
- SECURITY_AND_IMPLEMENTATION_PLAN.md (this doc)
- SECURITY.md (new)
- README.md (update short section)
Verification:
- Deployment documentation references concrete secret store and example commands.
Estimated effort: 0.5 day.

3) Session management and revocation strategy (P0)
Why: Stateless JWT cookie sessions are long-lived (7 days) and cannot be revoked; need token lifecycle and revocation to limit exposure.
Implementation steps:
- Shorten JWT lifetime (e.g., 15m) and implement a server-side refresh token mechanism stored hashed in DB or use opaque server sessions.
- Add refresh endpoint and rotate refresh tokens on use; support logout to revoke refresh token.
- Alternatively add a server-side session blacklist with expiration (rudimentary revocation) if refresh tokens are not desired.
Files/PR touches:
- internal/auth/* (new code for refresh tokens or session store)
- internal/database/store.go (new tables: sessions or tokens)
- internal/server/handlers.go (endpoints for refresh, revoke)
Verification:
- Simulate token theft and verify server-side revocation prevents access.
Estimated effort: 2-3 days.

4) Rate limiting and brute-force protection (P0)
Why: Protect auth endpoints and resource-heavy endpoints (uploads) from abuse.
Implementation steps:
- Add IP+account rate limiting middleware (in-memory, Redis-backed recommended for multi-instance); limit login/register/verify/otp send and upload endpoints.
- Implement exponential backoff, and account lockout after N failures with a cooldown.
Files/PR touches:
- internal/server/middleware.go (new rate-limit middleware)
- cmd/server/main.go (wire Redis or in-memory store via config)
Verification:
- Load tests and simulate repeated requests; check limits enforced and logs generated.
Estimated effort: 1-2 days.

5) Automated security scanning and static analysis (P0/P1)
Why: Catch common issues early (gosec, staticcheck, govulncheck).
Implementation steps:
- Add `gosec` and `govulncheck` runs in CI; add `golangci-lint` with staticcheck rules.
- Configure CI to fail on high-severity findings and report results as annotations.
Files/PR touches:
- .github/workflows/ci.yml (CI changes)
Verification:
- CI failing or warning when scanner finds issues; baseline exceptions reviewed.
Estimated effort: 0.5-1 day.

6) TLS enforcement and secure headers (P0)
Why: TLS must be enforced in production; HSTS and secure cookies required.
Implementation steps:
- Document recommended deployment: run behind TLS-terminating reverse proxy (Caddy/Traefik/NGINX) or use cert-manager on k8s.
- Set HSTS header in securityHeaders middleware when AppOrigin is HTTPS.
- Ensure auth.Manager sets Secure cookie flag already done; ensure SameSite and HttpOnly are set (already done). Consider setting cookie to SameSite=Strict for sensitive flows.
Files/PR touches:
- internal/server/middleware.go (HSTS addition conditional on secure origin)
- README.md (deployment guidance)
Verification:
- Deploy behind TLS locally using mkcert or test with a reverse proxy; verify headers and Secure cookie.
Estimated effort: 0.5 day.

7) CSRF defense (P1)
Why: Non-idempotent endpoints rely on Origin checks; add CSRF tokens for defense-in-depth.
Implementation steps:
- Add CSRF tokens for state-changing POST endpoints (use double-submit cookie or server-side token storage). Use the `sameOrigin` check as first line of defense and fallback to CSRF token verification for requests where Origin absent.
Files/PR touches:
- internal/server/middleware.go (csrf middleware)
- internal/server/handlers.go (verify tokens where needed)
Verification:
- Browser flows for state-changing requests continue to work; cross-site POSTs fail.
Estimated effort: 1 day.

8) Upload hardening (P0/P1)
Why: Uploaded videos are a vector for DOS, malware, and filesystem misuse.
Implementation steps:
- Continue using content sniffing; also verify container-level file storage path safety (already checks Base in Open).
- Add virus scanning hook (ClamAV) or cloud storage scanning on upload (async job).
- Enforce streaming and storage quotas per user; limit concurrent uploads.
- Add content-length enforcement and clean temp file handling (already uses temp file and renames).
Files/PR touches:
- internal/media/storage.go (hooks for async scan and quotas)
- internal/server/handlers.go (rate limiting + upload size already enforced)
Verification:
- Upload malformed files and verify rejection; simulate large uploads; verify quotas and scans run.
Estimated effort: 2-3 days (depending on scanning integration).

9) Database: migrations, backups, and production DB (P0/P1)
Why: SQLite is fine for dev but single-process; production should use a robust RDBMS and backups.
Implementation steps:
- Abstract DB layer to allow Postgres (use database/sql with driver). Add migrations using a migration tool (golang-migrate) and versioned SQL files.
- Document backup strategy and retention.
Files/PR touches:
- internal/database/* (migration scripts and optional DB driver configs)
- cmd/server/main.go and config to accept DB URL
Verification:
- Run migrations and smoke tests on Postgres; test backup/restore.
Estimated effort: 2-4 days.

10) Health, readiness, metrics, and logging (P1)
Why: Observability for deployments and debugging.
Implementation steps:
- Add /healthz and /ready endpoints returning 200 and DB connectivity respectively.
- Add Prometheus metrics (github.com/prometheus/client_golang) and basic counters (requests, errors, upload size, websocket connections).
- Ensure logs redact secrets and provide structured logs (JSON) optionally behind a flag.
Files/PR touches:
- internal/server/server.go (register endpoints)
- internal/server/middleware.go (metrics middleware)
Verification:
- Prometheus scrape works; health check behaves as expected.
Estimated effort: 1-2 days.

11) WebSocket origin and authentication checks (P0)
Why: WebSocket upgrader had CheckOrigin replaced per-connection; ensure allowed origins are verified and cookies used for auth.
Implementation steps:
- Ensure allowedOrigins map is strict and only configured origins allowed. Already used in websocket.Handle where localUpgrader.CheckOrigin uses allowedOrigins header value. Add explicit validation for the Origin header presence.
- Rate-limit websocket connections per IP.
Files/PR touches:
- internal/websocket/client.go (tighten Upgrader check and add logging)
Verification:
- Attempt WS connections from disallowed origins; ensure rejection.
Estimated effort: 0.5 day.

12) Container and runtime hardening (P1)
Why: Reduce attack surface and ensure resource limits.
Implementation steps:
- Keep distroless/nonroot base (already done); ensure image builds with nonroot user and no credentials baked in.
- Add resource limits, non-root checks in CI, and scan images (e.g., trivy) in CI.
- Add Dockerfile improvement: drop any stray tools, minimize layers, add HEALTHCHECK.
Files/PR touches:
- Dockerfile (tweak) and .github/workflows/ci.yml (add image scan)
Verification:
- Image scan passes with low/no findings; runtime runs as non-root.
Estimated effort: 0.5-1 day.

13) Access control reviews and tests (P1)
Why: Ensure handlers and DB queries enforce ownership and membership consistently.
Implementation steps:
- Add unit and integration tests for CreateRoom (owner only), VideoForRoomMember, Kick/Transfer, and room lock behaviors.
- Add property-based tests or fuzzing for boundary cases.
Files/PR touches:
- internal/database/store_test.go (add tests)
Verification:
- Test suite exercises edge cases and fails if access control breaks.
Estimated effort: 1-2 days.

14) Documentation and runbooks (P1)
Why: Clear guidance for deployers and operators improves security.
Implementation steps:
- Add SECURITY.md with incident response and secret handling.
- Add DEPLOYMENT.md describing reverse-proxy TLS setup, recommended TLS ciphers and HSTS, and scaling caveats.
Files/PR touches:
- SECURITY.md (new)
- DEPLOYMENT.md (new)
Verification:
- Docs reviewed and used by devs during deploy.
Estimated effort: 0.5-1 day.

15) Misc (P2)
- Add Dependabot (already planned).
- Add Sentry or similar error aggregation (optional).
- Add user email verification retries and rate limits.
Estimated effort: variable.

Next steps / sequencing recommendation
1. CI + security scanning + Dependabot (enables safe iteration) — P0
2. Secrets guidance and production env docs — P0
3. Rate limiting + brute-force protection — P0
4. Session revocation / refresh tokens — P0
5. Upload hardening (scan + quotas) — P1
6. DB migrations and production DB support — P1
7. TLS/HSTS, CSRF defenses, and metrics — P1
8. Container scanning and runtime hardening — P1
9. Tests, access control tests, and documentation — P1

If desired, implementation can start with an initial PR that adds CI and linters (item 1). Subsequent PRs should be small, each addressing one of the P0 items. For each PR, include:
- Description and rationale
- Files changed
- Test instructions
- Rollback notes

Would you like me to open PRs and implement item 1 (CI + scanners) now or start with another item? If you choose CI, confirm linters to run: golangci-lint or a smaller set (vet, staticcheck, govulncheck, gosec).