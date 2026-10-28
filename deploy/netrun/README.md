# Netrun: backend deployment

The backend is a Docker web application. Netrun detects `Dockerfile`, builds the
image and exposes the Go server on the `PORT` environment variable (the image
defaults to `8080`).

## One-time setup

1. Sign in at [netrun.io](https://netrun.io) and create a project from the public
   repository `https://github.com/q1est/IS_ej`, branch `main`.
2. Keep the project as a web application so Netrun assigns an HTTPS URL.
3. In the project secrets add:
   - `DATABASE_URL` — the Supabase PostgreSQL connection string (use the pooler
     connection string, `sslmode=require`).
   - `JWT_SECRET` — a random value of at least 32 bytes.
   - `ALLOWED_ORIGINS` — `https://q1est.github.io`.
4. Copy the HTTPS URL shown by Netrun and add it in GitHub under
   **Settings → Secrets and variables → Actions → Variables** as `NETRUN_API_URL`.
   The Pages workflow inserts it into `web/config.js`. Do not put database
   credentials in GitHub variables or in the frontend.
5. Copy the numeric Netrun project id and create these GitHub Actions secrets:
   - `NETRUN_API_KEY` — an access key from **Netrun → MCP**.
   - `NETRUN_PROJECT_ID` — the numeric project id.

After that, every push to `main` runs `.github/workflows/deploy-netrun.yml`.
It uploads only the tracked source archive and does not upload `.env` files or
runtime secrets. Netrun keeps `DATABASE_URL`, `JWT_SECRET`, and
`ALLOWED_ORIGINS` in its project settings; the published Pages site receives
only the public API URL.

The first publication must be created in the Netrun dashboard. The workflow is
for subsequent source updates from GitHub.
