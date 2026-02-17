# WIP — Session (T235)

## Current State

- **T200-T234**: Done (committed in prior sessions)
  - T233 skipped per T232 decision, T234 deferred to AI Orchestrator
  - Key commits: T230-T231=bd2caca, T232=1060c63, T234=b83bf5e
- **T235**: IN PROGRESS — Sync common config sub-feature

## T235 Design

### Feature: Shared config sync via sync repo

**New subcommands**: `osm sync config-push`, `osm sync config-pull`

**New config keys** (schema.go):
- `sync.config-sync` (bool, default false) — Enable config syncing
- `sync.config-sha` (string) — SHA256 of shared.conf at last sync (internal tracking)

**Sync repo structure** (`<sync-root>/`):
```
config/
  shared.conf          # Shared configuration (dnsmasq-format)
notebooks/
  YYYY/MM/...          # Existing notebook entries
```

**shared.conf format**:
```
# osm-shared-config-version 1
key value
...
```

**config-push behavior**:
1. Read local config Global options
2. Filter out sensitive/local-only keys (sync.*, log.file, session.*)
3. Write to `<sync-root>/config/shared.conf` with schema version header
4. Compute SHA256 of written content → store as `sync.config-sha`

**config-pull behavior**:
1. Read `<sync-root>/config/shared.conf`
2. Validate schema version (reject if unknown)
3. Check `sync.config-sha`:
   - No stored SHA → "unknown state" → require --force
   - Stored SHA matches remote file SHA → already applied, no-op
   - Stored SHA differs → remote changed, auto-apply (known state)
4. Merge remote keys into local config (in-memory only, report to user)
5. Update `sync.config-sha`

**Sensitive keys (never synced)**: Keys matching `sync.*`, `log.file`, `session.*`

**Schema version**:
- Line 1 of shared.conf: `# osm-shared-config-version 1`
- Higher version → error "shared config requires newer osm version"

## Files to Modify

- `internal/command/sync_config.go` — NEW: config-push, config-pull implementations
- `internal/command/sync.go` — Wire new subcommands in Execute dispatch
- `internal/config/schema.go` — Register new sync.* config keys
- `internal/command/sync_config_test.go` — NEW: comprehensive tests
- `docs/reference/config.md` — Document new keys

## Immediate Next Step

1. Create sync_config.go with config-push and config-pull
2. Wire into sync.go
3. Add schema keys
4. Write tests
5. make, Review Gate, commit
