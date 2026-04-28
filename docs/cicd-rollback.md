# CI/CD Rollback Runbook

## Scope

This runbook covers rollback operations when a deployment causes production issues.
Database rollback is not automated by pipeline.

## Preconditions

- Confirm incident impact and open an incident ticket.
- Identify the latest known good backend image tag in GHCR.
- Confirm whether issue is frontend-only, backend-only, or both.

## Frontend Rollback (Vercel)

1. Open Vercel project dashboard.
2. Select Deployments.
3. Promote a known good deployment to production.
4. Verify:
   - frontend app loads
   - login works
   - room create/join/start works

Alternative:

- Use workflow dispatch: `Rollback` (`.github/workflows/rollback.yml`)
- Set `frontend_deployment_url` to the previous Vercel deployment URL.

## Backend Rollback (GHCR image)

1. Select previous known-good image tag:
   - `ghcr.io/<org>/blackjack-backend:<tag>`
2. Redeploy backend infrastructure with that image tag.
3. Verify:
   - `/health` responds 200
   - API authentication works
   - WebSocket auth and sync work

Automation path:

- Configure:
  - `STAGING_BACKEND_ROLLBACK_WEBHOOK_URL`
  - `PRODUCTION_BACKEND_ROLLBACK_WEBHOOK_URL`
- Trigger `Rollback` workflow with `backend_image_tag`.

## Migration Failure Handling

- If migration fails during pipeline:
  - stop deployment immediately
  - keep currently running app version
  - investigate migration SQL and schema drift
- Do not apply automatic down migrations in production unless explicitly reviewed and approved.

## Post-Rollback Checks

- Run smoke checks:
  - `/health`
  - `/api/me`
  - room create
  - websocket `AUTH` + `ROOM_STATE_SYNC`
- Confirm error rate, reconnect count, and latency are back to baseline.

When using `Rollback` workflow:

- The workflow automatically runs post-rollback smoke checks for:
  - health endpoint
  - API auth + room creation
  - websocket auth/sync and HTTP-vs-WS version consistency
- The workflow writes a `Rollback Summary` section to the run summary.
- Slack notification:
  - success notification on completed rollback
  - failure notification with `failed_jobs=[...]` when rollback steps fail

## Follow-up

- Capture root cause and timeline.
- Add a regression test (CI integration or smoke).
- Update deployment checklist if gaps were found.
