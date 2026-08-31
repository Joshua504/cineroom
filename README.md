# Cineroom

Cineroom is a private, invite-based watch-room application for synchronizing video playback and chatting with other authenticated users.

## Run locally

```sh
cp .env.example .env
export APP_SECRET='replace-this-with-at-least-32-random-characters'
go run ./cmd/server
```

The `.env` file is local-only and must never be committed. Load it with your preferred environment tool or export its values before starting the server.

Open <http://localhost:8080>. Data and uploaded videos are stored under `DATA_DIR`.

## Google sign-in

Set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`, and register this callback URL with Google:

```text
http://localhost:8080/api/auth/google/callback
```

For production, set `APP_ORIGIN` to the HTTPS origin and register the corresponding callback URL.

## Email verification

Password registration sends a six-digit code that expires after 10 minutes. Configure an SMTP server with `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, and `SMTP_FROM`. Accounts are not created until the code is verified.

If `SMTP_HOST` is left empty, the app automatically runs in local development mode and prints the OTP to the server logs instead of trying to send an email. This is convenient for local work, but production deployments should still configure real SMTP credentials.

For Gmail SMTP, enable 2-Step Verification, create a Google app password, then use `smtp.gmail.com`, port `587`, your Gmail address as `SMTP_USERNAME` and `SMTP_FROM`, and the app password as `SMTP_PASSWORD`.

## Docker

```sh
docker build -t cineroom .
docker run --rm -p 8080:8080 \
  -e APP_SECRET='replace-this-with-at-least-32-random-characters' \
  -v cineroom-data:/data cineroom
```

Run tests with `go test ./...`.

## Secrets & production

Do not commit secrets or a `.env` file into the repository. The `.env.example` file documents required variables; in production, store and inject secrets using a secrets manager or platform-native secret mechanism (e.g., HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, Kubernetes Secrets, Docker Secrets, or GitHub Actions secrets).

Recommended minimal production configuration:
- APP_SECRET: a random secret of at least 32 characters (required). Rotate regularly.
- APP_ORIGIN: the HTTPS origin (e.g. https://example.com).
- GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET: when enabling Google sign-in.
- SMTP_* values: when sending verification emails.

Example Docker Secrets (Docker Swarm):
- Create a secret: `echo -n "$(cat .env | grep APP_SECRET | cut -d '=' -f2-)" | docker secret create app_secret -`
- Run container with secret: `docker service create --name cineroom --secret app_secret ...`

For CI/CD pipelines, store secrets in the provider's secret storage and inject them as environment variables at deploy time. Keep a rotation and revocation plan (who can access secrets, how to rotate, and how to revoke compromised secrets).

See SECURITY.md for more guidance.

