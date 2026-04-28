# CI/CD Setup

## Workflows

- CI: `.github/workflows/ci.yml`
  - Trigger: `pull_request`
  - Runs quality gates for frontend/backend/integration/container/security.
  - Integration includes reconnect and rematch both branches (agree=false and agree=true).
- CD: `.github/workflows/cd.yml`
  - Trigger: `push` to `main`, `workflow_dispatch`
  - Flow: require CI success -> verify secrets -> build/push image -> staging migration -> staging deploy -> staging smoke -> production migration -> production deploy.
- Preflight: `.github/workflows/cicd-preflight.yml`
  - Trigger: `workflow_dispatch`, weekly schedule
  - Validates required secrets and health endpoints before first release run.
  - Also validates secret format conventions (`postgres://`, `http(s)://`, `ws(s)://`).
  - Writes `CI/CD Preflight Summary` and can notify success/failure to Slack.
- Load test: `.github/workflows/load-test.yml`
  - Trigger: `workflow_dispatch`
  - Runs k6 against selected target (`staging` or `production`).
  - Writes `Load Test Summary` and can notify success/failure to Slack.
- Rollback: `.github/workflows/rollback.yml`
  - Trigger: `workflow_dispatch`
  - Executes backend rollback webhook and optional Vercel frontend promote.
- Governance check: `.github/workflows/governance-check.yml`
  - Trigger: `workflow_dispatch`, weekly schedule
  - Validates `main` branch protection and `production` environment reviewer policy.
  - Writes `Governance Check Summary` and can notify success/failure to Slack.
  - Validates: PR review required, approving review count >= 1, force-push disabled, deletion disabled, strict required checks enabled.

## Required Secrets

Repository secrets:

- `VERCEL_TOKEN`
- `VERCEL_PROJECT_ID`
- `VERCEL_ORG_ID`
- `STAGING_DATABASE_URL`
- `STAGING_BACKEND_HEALTHCHECK_URL`
- `STAGING_API_BASE_URL` (example: `https://api-staging.example.com/api`)
- `STAGING_WS_BASE_URL` (example: `wss://api-staging.example.com/ws`)
- `PRODUCTION_DATABASE_URL`
- `PRODUCTION_BACKEND_HEALTHCHECK_URL`
- `PRODUCTION_API_BASE_URL` (example: `https://api.example.com/api`)
- `PRODUCTION_WS_BASE_URL` (example: `wss://api.example.com/ws`)
- `SLACK_WEBHOOK_URL` (optional: notification for CI/CD failures)
- `STAGING_BACKEND_ROLLBACK_WEBHOOK_URL`
- `PRODUCTION_BACKEND_ROLLBACK_WEBHOOK_URL`

## GitHub Environments

Create two environments:

- `staging`
- `production`

For `production`, enable Required reviewers to enforce manual approval before production migration and deploy jobs.

## Branch Protection

For `main`:

- Require pull request before merging.
- Require status checks to pass before merging.
- Add `CI` workflow checks as required.
- Run `Governance Check` workflow to validate these policies continuously.

Recommended required job checks (from CI workflow):

- `backend`
- `frontend`
- `integration`
- `migration-compatibility`
- `container-build`
- `security`

## Smoke Test Expectations

Staging smoke tests in CD validate:

- Frontend URL responds.
- Backend health endpoint responds.
- Backend API signup/login token flow works.
- `/api/me` succeeds.
- Room creation succeeds.
- WebSocket `AUTH` succeeds and `ROOM_STATE_SYNC` is received.
- Second WebSocket connection also authenticates/syncs (reconnect path smoke).
- `ROOM_STATE_SYNC` payload consistency is checked (`room.id` match and numeric `session.version`).
- `ROOM_STATE_SYNC.session.version` is compared with HTTP `GET /rooms/{id}` version for state consistency.

Production smoke tests in CD validate:

- Frontend deployment URL responds after production deploy.
- Backend health endpoint responds after production deploy.
- API signup/token flow works.
- `/api/me` succeeds.
- Room creation succeeds.
- WebSocket `AUTH` succeeds and `ROOM_STATE_SYNC` is received.
- `ROOM_STATE_SYNC` payload consistency is checked (`room.id` match and numeric `session.version`).
- `ROOM_STATE_SYNC.session.version` is compared with HTTP `GET /rooms/{id}` version for state consistency.

## Failure Notifications

- `CI` and `CD` workflows include a `notify-*-failure` job.
- If `SLACK_WEBHOOK_URL` is configured, failures post a message with run URL.
- Notification message includes failed job names (`failed_jobs=[...]`).
- Notification message also includes `actor` and `sha`.
- If not configured, notification step is skipped without failing the pipeline.
- Rollback workflow also posts failure notification when configured.
- CD also posts success notification when enabled (`notify-cd-success`).

## Workflow Summary

- CD writes a step summary section (`CD Summary`) with deploy/smoke results and deployment URLs.
- Rollback writes a step summary section (`Rollback Summary`) with rollback step results.

## Security Reports

- `CI` security job stores reports as an artifact (`security-reports`).
- Included files:
  - `gosec.txt`
  - `gosec.sarif`
  - `trivy.txt`
  - `npm-audit.txt`
- `gosec.sarif` is also uploaded to GitHub Code Scanning (Security tab).

## Quality Gate Coupling

- CD includes `require-ci-success` to verify the `CI` workflow already passed for the same commit SHA.
- If CI is missing or not successful, CD fails before any migration/deploy step.

## Load Test

- Use `Load Test` workflow dispatch.
- Input:
  - `target`: `staging` or `production`
  - `vus`: default `100`
  - `duration`: default `5m`
  - `ws_ratio`: default `0.3`
  - `ws_reconnect_ratio`: default `0.5`
- Input validation:
  - `vus` must be positive integer
  - `duration` format: `<number><s|m|h>`
  - `ws_ratio` and `ws_reconnect_ratio` must be between `0` and `1`
- Script path: `tests/load/k6-smoke.js`
- Mixed scenario:
  - HTTP (`signup` -> `/me` -> `room create` -> `join`)
  - WS (`AUTH` -> `ROOM_STATE_SYNC`) with adjustable mix ratios
- Thresholds:
  - `http_req_duration p95 < 500ms`
  - `http_req_failed rate < 5%`
  - `ws_sync_success rate > 95%`
  - `ws_sync_latency_ms p95 < 100ms`
- Workflow summary:
  - `Load Test Summary` is written to the Actions run summary.
- Failure notification:
  - If `SLACK_WEBHOOK_URL` is configured, load test failure sends a Slack notification.
