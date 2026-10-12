# Maintenance restart verification — 2026-10-12

Maintenance restarts now have the same 20-minute readiness deadline as ordinary
starts. An unconfirmed restart finishes its job as failed and retains a recovery
hold and the owned process, rather than leaving the job active indefinitely.
Supervisor stop, exit and failed-restart updates serialize their ship snapshots
so an exit cannot overwrite the recovery state.

## Automated regressions

`internal/jobs/maintenance_restart_test.go` exercises the actual scheduler,
durable queue, executor, supervisor, resource admission and subprocess tracking,
with helper processes substituting for Vere:

- Scheduled Pack → Meld → Roll → Chop completes each utility, restores the
  previously running ship, records its restarted writer and releases the queue.
  Subsequent manual Stop and Start both complete.
- A successful Pack followed by a private-control route returning HTTP 500 hits
  a short test readiness deadline. Its job becomes failed, with no pending job;
  the ship leaves starting, while its recovery hold, utility exit status and
  live writer/guard remain intact.
- The held state survives the owned runtime's subsequent graceful exit. Later
  scheduled stages settle behind the hold without launching utility processes.

Both failure assertions were observed failing before their fixes. Focused race
tests and the complete `go test -race -count=1 -p 1 ./...` suite passed on
`darwin/arm64`, followed by `go vet ./...`. A first complete invocation alongside
native runtime startup failed `TestCurrentRuntimeUpdateLeavesRunningShipUndisturbed`
with an early helper exit; the complete serialized rerun passed. Runtime helper
fixtures and the native trial were then run separately. No assertions or checks
were disabled.

Frontend build, typecheck and unit tests, the nine native-acceptance harness
tests, four packaging tests, module downloads, notice collection and diff checks
also passed.

## Native scheduled cycle

A locally built runner, SHA-256
`38f31414ab21d6bcb41c71f04ab5573396169c43dce3eb77be96220fc560a9b4`,
booted a fresh disposable comet with official Vere 4.6 and a 2 GiB loom on an
Apple Silicon Mac. The isolated registry used a port-zero control listener and
loopback ship HTTP binding. The operator's registered pier and schedules were
not used.

The trial edited only the comet's schedules through the production API and let
the normal scheduler enqueue the occurrences. Paired Chop was scheduled after
Roll. The resulting scheduled Pack, Roll and Chop all completed with utility
exit 0, recorded their automatic restarts, and left the ship running with no
pending job or recovery hold. Chop depended on that cycle's successful Roll.

Scheduled Meld skipped normally because the fresh snapshot was below its
512 MiB pressure threshold. A separate manual Meld completed and restarted the
same comet, providing native coverage of that utility's shared restart path.
Helper regressions cover the admitted scheduled-Meld branch; this native trial
does not claim a large-snapshot automatic Meld ran.

Authenticated login passed after maintenance. A subsequent manual Stop/Start
cycle also completed, followed by the standard native Pack/Roll/Chop,
export/restore and replay checks. Both registered disposable piers finished
stopped, and their manager exited 0.

The first schedule setup attempt was refused because moving Roll's clock time
past its paired Chop violated the existing pairing rule. The trial then resumed
the same freshly created, gracefully stopped comet with corrected edit order;
it did not substitute an older archived identity or bypass the refusal.

This establishes native scheduled-restart behavior on this disposable
`darwin/arm64` fixture. It does not certify every populated ship, native Intel,
physical storage failure, or a genuine 20-minute stalled Vere restart.
