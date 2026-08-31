Security guidance for Cineroom

This file contains guidance for handling secrets, incident response basics, and secure deployment notes.

1. Secrets & sensitive configuration
- Never commit secrets (APP_SECRET, OAuth client secrets, SMTP credentials, database credentials, API keys) to version control.
- Use a secrets manager for production: HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, or the secret facility of your platform (Kubernetes Secrets, Docker Secrets).
- In CI systems, use encrypted repository secrets (GitHub Actions Secrets, GitLab CI Variables) and never echo them into logs.
- Rotate secrets periodically and after any suspected compromise.

2. Local development
- Use `.env` files only for local development. Keep `.env` in `.gitignore` (already present). Use `.env.example` to document values.
- For shared development, use a tool that supports encrypted environment files or a central vault.

3. Access control and least privilege
- Limit access to secrets to only those roles that need them.
- Use short-lived credentials where possible and audit access.

4. Infrastructure and TLS
- Always terminate TLS in production. Use a well-known reverse proxy or automated TLS solutions (Caddy, Traefik, cert-manager + ACME).
- Enforce HSTS and safe TLS ciphers. Set Secure and HttpOnly on cookies (the application already sets these), and prefer SameSite=Lax/Strict where suitable.

5. Incident response (brief)
- If a secret is suspected compromised, rotate it immediately and invalidate tokens/sessions where possible.
- Keep a runbook for revoking access and restoring services.

6. Reporting vulnerabilities
- For any security concerns, open an issue in the private security tracker or contact the maintainers directly. Do not post secrets publicly.

7. Additional recommendations
- Add automated security scanning (gosec, govulncheck) to CI.
- Run container image scanning (Trivy) on release images.
- Add rate-limiting and account lockout to prevent abuse.

This document is a minimal guide for maintainers. Follow your organization's security policies for production deployment.
