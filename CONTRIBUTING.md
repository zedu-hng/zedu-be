# Contributing to Zedu Backend

Thanks for contributing to the Zedu backend (`telex_be`). This guide is the source of truth for how
work is proposed, reviewed and merged. For the detailed Go engineering standards see
[`AGENT.md`](./AGENT.md); for AI coding agents see [`AGENTS.md`](./AGENTS.md).

## Table of contents

- [Ways to contribute](#ways-to-contribute)
- [Before you start](#before-you-start)
- [Repository structure](#repository-structure)
- [Branching model](#branching-model)
- [Commit conventions](#commit-conventions)
- [Opening a pull request](#opening-a-pull-request)
- [Testing requirements](#testing-requirements)
- [Code standards](#code-standards)
- [Feature flags](#feature-flags)
- [Database changes](#database-changes)
- [Protected files](#protected-files)
- [Code review](#code-review)
- [Bootcamp roles and expectations](#bootcamp-roles-and-expectations)
- [Security](#security)
- [Using AI tools](#using-ai-tools)
- [Questions](#questions)

## Ways to contribute

1. Search existing issues before opening a new one.
2. Open an issue to discuss larger changes before implementing.
3. Pick up a ticket, branch from `dev`, and open a pull request.
4. Keep changes small and focused — one feature or bug fix per branch/PR.

## Before you start

- Go 1.24 or newer.
- Docker and Docker Compose (recommended for the full local stack).
- Access to the repository and a GitHub account added to the org.
- The `migrate` CLI on your `PATH`, used by the `make migrate-*` targets:

  ```sh
  go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1
  export PATH="$(go env GOPATH)/bin:$PATH"
  ```

Set up your environment:

```sh
cp app-sample.env app.env                          # app.env is gitignored
docker compose -f docker-compose.dev.yml up --build # full local stack (or: make start-dev)
```

The API is then available at http://localhost:8019 and Swagger at
http://localhost:8019/api/docs/index.html. See [`dev-setup.md`](./dev-setup.md) for troubleshooting and
running against services already provisioned elsewhere.

## Repository structure

Requests flow through a fixed path:

```
pkg/router  ->  pkg/controller/<feature>  ->  services/<feature>  ->  pkg/repository/storage
```

- `pkg/controller/<feature>/` — Gin handlers (bind, validate, respond).
- `services/<feature>/` — business logic, one package per feature.
- `internal/models/` — domain structs, DTOs and schema mappings.
- `pkg/repository/storage/` — storage adapters (postgresql, elastic, redis, minio, mongodb, typesense).
- `internal/config/` — env loading and `BaseConfig`.
- `tests/test_<feature>/` — **all** tests live here (never inside `internal/` or `pkg/`).

## Branching model

- `dev` — integration branch. All bootcamp PRs target `dev`.
- `central-staging` — promotion/staging branch, updated by leads from `dev`.
- `main` — production.

**Never push directly to `dev`, `central-staging` or `main`.** Every change goes through a pull request.

Branch names use `<type>/<TICKET-ID>-<short-description>`, where `<type>` matches the commit type:

```
feat/ZED-142-channel-pins
fix/ZED-201-nil-thread-dm-list
docs/ZED-155-contributing-guide
```

Keep branches short-lived. Rebase on `dev` before opening or updating a PR if needed.

## Commit conventions

Use [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): description`.

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `perf`, `ci`, `chore`, `style`, `build`, `revert`.

```
feat(channel): add channel pins endpoint

fix: guard nil thread on DM list
```

- Use an imperative, concise subject line; keep the type-based prefix in lowercase.
- Reference the ticket when relevant (e.g. `Refs: ZED-142` or in the PR body).
- Keep commits small and logically grouped. Do not merge unrelated changes into one commit.

## Opening a pull request

- Open the PR against `dev`.
- **At most one open PR per author.** Get the previous one merged before opening the next.
- Keep the PR small and atomic; large PRs are harder to review and easier to break.
- Fill the PR template completely. The **ticket ID is mandatory**.
- Link the related issue where applicable (e.g. `Closes #123`).
- Add the requested reviewer(s) — see [Code review](#code-review). Assign yourself as the assignee.
- Ensure automated checks pass before requesting review; address failures on your branch.
- Do not open a PR for a branch that does not build or has failing tests.

## Testing requirements

- Every test file lives in `tests/test_<feature>/` with package `test_<feature>`
  (e.g. `package test_message`). Do not add tests inside `internal/` or `pkg/`.
- Add or update tests for any behaviour you change, even if nobody asks.
- Run the suite locally before pushing:

```sh
go build ./...
go vet ./...
go test ./tests/... -p 1 -timeout 600s   # -p 1 avoids GORM auto-migrate deadlocks
```

- If your change touches migrations, exercise it against a fresh database where feasible.
- A red build or failing test suite is not "done". Report the exact commands you ran.

## Code standards

- Follow Go idioms and keep `gofmt`/`goimports` clean.
- Read [`AGENT.md`](./AGENT.md) for layering, the 3-parameter rule, response builders, storage
  wrappers and comment discipline. [`AGENTS.md`](./AGENTS.md) is the concise agent-facing summary.
- Use the PostgreSQL and Elastic wrappers rather than ad-hoc queries.
- Never hardcode secrets, URLs, ports or keys — add settings to `BaseConfig` and `app-sample.env`.

## Feature flags

- Wrap new user-facing behaviour in a **feature flag** that defaults to **OFF**.
- Keep the flag name and default documented in the PR so reviewers can verify the safe path first.

## Database changes

- Follow **expand → contract**: ship additive migrations first (new columns/tables, backfill), remove the
  old shape in a later PR once nothing depends on it.
- Never rewrite or delete an already-applied migration.
- Use the migration commands from the root `Makefile`:

```sh
make migrate-safe-up
make migrate-down
```

## Protected files

These files affect the whole project and require **lead approval** before changing:

- `.github/`
- `Dockerfile`, `Dockerfile.dev`
- `docker-compose*.yml`, `.air.toml`
- `Makefile`
- `AGENTS.md`, `AGENT.md`, `CONTRIBUTING.md`
- `internal/config/`

If your ticket genuinely requires one of these, call it out explicitly in the PR and wait for approval.

## Code review

- Reviewers are routed by team; the PR validator bot and GitHub will request the appropriate reviewer(s).
- At least one reviewer approval is required; changes to protected files need a lead's approval.
- Address every review comment or reply explaining why it is not applicable.
- Keep discussions on the PR. Push follow-up commits to the same branch; avoid force-pushing after a
  review has started.
- Reviewers: be specific, focus on correctness, security and maintainability, and re-review promptly.

## Bootcamp roles and expectations

Promotion flows Interns → Team Leads → Reviewers → In-house maintainers.

- **Interns** — pick up scoped tickets, open one small PR at a time, respond to review quickly.
- **Team Leads** — triage tickets, keep PRs unblocked, escalate protected-file and design questions.
- **Reviewers** — review assigned PRs, enforce these rules, request changes when needed.
- **In-house maintainers** — approve protected changes, manage `central-staging`/`main`, merge.

## Security

- Validate and sanitise all external input before use.
- Use parameterised GORM queries; never concatenate raw SQL.
- Keep secrets out of source, logs and commits. `app.env` must never be committed.
- Treat authentication, payments (Stripe) and credential handling as sensitive.
- Report vulnerabilities privately to the maintainers — do not open a public issue.

## Using AI tools

- AI coding agents should read [`AGENTS.md`](./AGENTS.md) and [`AGENT.md`](./AGENT.md) before making changes.
- AI-assisted is fine, but **you are accountable** for the result: review, test and understand every change
  before requesting review.
- Never paste secrets, tokens or customer data into external tools.
- Ensure generated code follows the same standards and test requirements as hand-written code.

## Questions

If you are unsure where to start, check [`README.md`](./README.md), [`dev-setup.md`](./dev-setup.md) or
open an issue asking for guidance. Thanks for helping improve the project!
