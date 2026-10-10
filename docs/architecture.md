# Architecture

Harbor is a single-user Go process serving an embedded React application and `/api/v1`. SQLite stores ships, schedules, durable jobs, launch identities, output and recovery holds. Closing a browser does not end a ship. Background installers register this same process with the OS user service manager.

| Area | Responsibility |
| --- | --- |
| `cmd/harbor` | Configuration, volume guards, recovery, listener and graceful shutdown. |
| `internal/onboarding` | Key/comet boot, stopped-folder registration and archive restore. |
| `internal/supervisor` | Owned Vere processes, readiness, PTY and process identity. |
| `internal/jobs` | Serialized maintenance/export/replay/update with durable phases. |
| `internal/scheduler` | Calendar/DST occurrences, resource profiles and exact Roll/Chop dependencies. |
| `internal/backup`, `internal/pier`, `internal/storage` | Full archives, canonical paths/writer locks and startup filesystem identity. |
| `internal/runtime` | Native runtime catalog/download verification, arguments and private HTTP control. |
| `internal/server`, `web/src` | Control API, browser UI, events and terminal. |
| `internal/store` | SQLite records and transactions. |
| `scripts`, `packaging` | Build/notices/source bundles and per-user service registration. |

Runtime artifacts come from a fixed official Vere catalog with pinned SHA-256 digests and native architecture checks. The selected stable runtime is currently 4.6. The access-code query uses fixed private loopback HTTP, disables proxies/redirects and bypasses ordinary persisted job logs. The Dojo is a separate, deliberately privileged output/input channel.

On all four supported macOS/Linux targets, every ship launch selects an LMDB virtual mapping to mitigate Vere 4.6's default-map/loom collision. After path and ownership checks, the supervisor measures the largest metadata or epoch database by logical file length and selects at least twice that size, rounded up to a power of two with a 64 GiB minimum. Epochs are not summed; new piers use 64 GiB. The choice is recalculated on each start, with no live resize or 32 GiB database cutoff. The utility executor remeasures under the pier guard for Pack/Meld/Roll/Chop and places the map option after the subcommand. Replay (`play`) and update preparation (`prep`) do not accept that option in Vere 4.6; their subsequent ship restarts use the supervisor policy. This does not reserve or protect loom addresses, repair corrupt databases or guarantee that larger maps fit a host. The [workaround maintenance note](lmdb-mapping-workaround.md) defines exact coverage, admission checks, evidence, limits and retirement gates tied to the [upstream address-ownership proposal](upstream/vere-loom-reservation-proposal.md).

Before starting work, Harbor recovers durable job state and preserves uncertain outcomes as holds. It signals only processes it owns after launch identity checks. External surviving writers are not silently adopted. One executor serializes utility jobs; browser actions queue rather than bypass this ownership model.

The manager's `dataDir` contains SQLite, runtimes, uploads and backups. `allowedRoots` restricts pier paths but does not sandbox Vere or isolate clients/ships into OS accounts. The service and its children run with the service user's authority. Every admitted control client shares management authority; see the [security policy](../SECURITY.md).

The visual preview at `/preview.html` uses simulated data and localStorage. Production uses the real API. Preview tests demonstrate layout/state handling, not native ship behavior. Upstream `@urbit/sigil-js` renders supported short identities; unsupported identity classes get an explicit fallback rather than an invented sigil.
