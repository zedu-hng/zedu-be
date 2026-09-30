## Ticket
<!-- Mandatory. The ticket/issue this PR addresses, e.g. ZED-142 -->
Ticket:

## Summary
<!-- What does this PR do, and why? Keep it short and specific. -->

## Type of change
- [ ] `feat` — new feature
- [ ] `fix` — bug fix
- [ ] `docs` — documentation only
- [ ] `refactor` — no behaviour change
- [ ] `test` — tests only
- [ ] `perf` — performance
- [ ] `ci` — CI/workflows
- [ ] `chore` — maintenance
- [ ] `style` — formatting only
- [ ] `build` — build system/dependencies
- [ ] `revert` — revert of a previous change

## Related issue
<!-- e.g. Closes #123 -->
Closes #

## How has this been tested?
<!-- Give the exact commands and paste evidence (logs, curl output, screenshots). -->

- [ ] Manual
- [ ] Unit
- [ ] Integration

**Environment**
- Go version:
- Database:
- OS:

## Checklist
- [ ] This PR is small and atomic (one feature or bug fix)
- [ ] Branch name follows `<type>/<TICKET-ID>-<short-desc>`
- [ ] PR title follows conventional commits (`type(scope): description`)
- [ ] I have only this one open PR (one open PR per author)
- [ ] New user-facing behaviour is wrapped in a feature flag, default **OFF**
- [ ] Database changes follow **expand -> contract**
- [ ] Tests added or updated under `tests/test_<feature>/`
- [ ] `go build ./...` passes locally
- [ ] `go test ./tests/... -p 1 -timeout 600s` passes locally
- [ ] No secrets, keys or `app.env` values are included

## Feature flag
<!-- Name and default, or "N/A" -->
Flag:
Default: OFF

## Database / migrations
<!-- Migration files, backfill notes and rollback plan, or "N/A" -->

## Protected files
<!--
Changing any of .github/, Dockerfile*, docker-compose*.yml, .air.toml, Makefile, AGENTS.md,
AGENT.md, CONTRIBUTING.md or internal/config/ requires lead approval. List them here.
-->

## Screenshots / evidence
<!-- Postman results, terminal output, logs, etc. -->

## Breaking changes
- [ ] No
- [ ] Yes — described below

**If yes, describe the breaking changes and migration path:**
