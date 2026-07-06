# M7 — Cloud layer (DEFERRED — build only when the user explicitly asks)

**Goal:** sharing beyond the repo, per DESIGN.md §9: self-hosted on the user's provider account, capability-link auth, git remains truth. This file is a full spec so the work can start cold, but **v0.1 done ≠ M7 started.**

**Depends on:** M6.

## Components

### 1. `pkg/publish` — provider interface (in-tree from the start)

The interface from DESIGN.md §9.2, plus:

```go
type ProviderConfig struct{ Provider string; Account map[string]string } // provider-specific creds/ids
type Deployment struct{ APIBase, SiteBase string; AgentKey string }      // written to ~/.config/quibble/<provider>.yml (0600)
type Registry map[string]func() Publisher                                // "cloudflare", "aws" register here
```

- Remote comment API contract (implemented by every provider's backend): mirrors `store.CommentStore` verbs over REST — `GET/POST /v1/threads`, `POST /v1/threads/{id}/replies`, `POST /v1/threads/{id}/status` — with the same JSON field names as CLI `--json` (frozen in M4). Conformance: `storetest.Run` executes against a `remoteStore` adapter that wraps the HTTP API pointed at a local test double; each provider's deployed backend is smoke-tested with the same suite via an opt-in env-gated test (`QUIBBLE_E2E_CLOUDFLARE=1`).

### 2. Commands

- `quibble cloud setup --provider cloudflare` — interactive: provider API token in, resources provisioned (idempotent re-run = verify + repair), `Deployment` saved. `cloud status`, `cloud teardown`.
- `quibble publish` — render + upload site; push current threads. Project config gains `publish: {provider: cloudflare, project: <name>}`.
- `quibble sync pull [--since]` — fetch remote threads/replies; merge into fs store. Merge rules: remote thread unknown locally → create; known → append missing replies (dedupe by author+time+body hash); status conflicts → **local wins**, warn (git is truth); orphan-doc threads (doc deleted locally) → still import, doctor flags.
- `quibble share <doc> --with <name>` / `share list` / `share revoke <name>` — mint/list/revoke signed capability tokens (HMAC over name+doc+exp with a server-side secret from setup; no JWT dep needed).

### 3. Cloudflare adapter (first provider)

- Site → Pages (or Workers static assets); comments API → one Worker; storage → D1 with a 2-table schema (threads: full YAML-equivalent columns; replies). Worker code in `adapters/cloudflare/worker/` (TypeScript is acceptable here — it runs on workerd, not in the Go binary; keep it dependency-light and generated wrangler config committed).
- Auth in the Worker: agent key (header) = full CRUD minus resolve-as-human; capability token (query param → cookie) = read + create/reply attributed to the token's name; no token = 404 (not 401 — don't advertise existence).
- Reader URLs: `<SiteBase>/<random 22-char slug>/<doc-slug>` — the random segment is the read capability.

### 4. Viewer UI

Reuse the M5 frontend verbatim against the remote API (the API shapes are identical by design). The published site embeds the same `comments.js` with a different endpoint base — this symmetry is a hard requirement; if M5's UI can't run remote-unmodified, fix M5 rather than forking.

## Tests

| # | Case | Expect |
|---|------|--------|
| 1 | `storetest.Run` vs remoteStore + in-process test double | green, unmodified suite |
| 2 | sync pull: new remote thread | file appears in `.quibble/comments/` |
| 3 | sync pull: reply dedupe | pulling twice → no dup replies |
| 4 | sync pull: status conflict | local status kept, warning printed |
| 5 | share token: valid/expired/revoked/forged | 200 / 401 / 401 / 401; attribution = token name |
| 6 | no credential | API returns 404 |
| 7 | capability URL rotation | old URL 404s, new serves |
| 8 | `cloud setup` idempotency | second run repairs/no-ops, never duplicates resources |
| 9 | env-gated live smoke (`QUIBBLE_E2E_CLOUDFLARE=1`) | publish → comment via capability link → sync pull lands the file |

## Acceptance

- From zero: `cloud setup` → `publish` → send a share link from another browser/incognito → comment as "sam" → `sync pull` → thread file in git with `author: sam` → agent addresses it via CLI → `publish` → sam sees the reply. The whole loop, no accounts anywhere.
