# Native acceptance trials

Cross-compilation and Rosetta execution do not establish native Intel support. Run the opt-in acceptance script on the actual target, from a built source checkout with Python 3 and access to the official Vere runtime/pill hosts:

```sh
python3 scripts/native-acceptance.py \
  --binary dist/harbor --output /absolute/new-trial-directory \
  --disposable --require-platform linux/amd64 --lifecycle
```

Choose `darwin/amd64`, `darwin/arm64`, `linux/amd64` or `linux/arm64` for the actual host. The platform guard rejects an ARM Mac when Intel acceptance is requested, including an Intel Python process under Rosetta. The output directory must not already exist. Harbor uses new manager state and loopback listeners; the script never chooses an operator's existing pier/configuration.

The default mode boots a fresh comet through Harbor's production API. Keyfile trials additionally require a **disposable, valid boot key** and its identity:

```sh
python3 scripts/native-acceptance.py \
  --binary dist/harbor --output /absolute/another-new-trial-directory \
  --disposable --require-platform darwin/arm64 \
  --mode keyfile --key /private/disposable.key --identity '~sampel-palnet'
```

Never supply the key or pier of a real existing identity. The key path stays on the trial host. The script streams uploads privately, checks staged-key removal, retrieves an access code in memory, performs HTTP login, and confirms the identity in the authenticated session. It writes credential-free `status.json`; service/ship logs and piers remain private local evidence.

Basic acceptance includes the assigned port-zero listener, empty registry, native platform match, boot, private query/login, graceful stop and restart. `--lifecycle` also exercises Pack, Roll, Chop, verified stopped-source export, streamed download/upload, restored native boot, a checkpoint belonging to the restored ship, replay and login. Source and destination run sequentially. The script does not certify Meld, a runtime version transition, physical failures, OS service registration or real-identity migration. Those require separate trials.

Failures retain all trial state. Harbor waits for owned native runtimes to exit gracefully, retaining its PTY, database and process tracking while bootstrap finishes. A caller's canceled stop leaves the child owned. The acceptance harness's final 90-second manager wait can record `shutdownTimedOut`; it does not force-kill the ship or prove the controller has stopped. Inspect `status.json`, job history and private output; never overwrite/delete a failed pier to make acceptance pass. Check for surviving writers before reusing a trial. First comet bootstrap can take longer than its later restarts; a successful login does not certify every early shutdown window.

Fresh comet acceptance depends on access to the official bootstrap pill. A trial that cannot download it has not exercised LMDB startup or authenticated onboarding. Testing a retained disposable comet is supplemental lifecycle evidence and must be labeled separately.

Login acceptance requires a newly issued session cookie and the expected `window.ship` assignment from the exact same-origin `/session.js` URL with a JavaScript MIME type. Redirected login HTML can contain the public ship identity and is not authenticated proof. Fresh app routes may initially return 404; only that response is retried, for at most two minutes. A missing landing page does not by itself establish a failed login. Run the isolated HTTP contract regressions with `python3 scripts/test_native_acceptance.py`.

Keep retained comet fixtures current and run one copy of an identity at a time. A checksum-valid older export can still be refused by Vere with `double-boot detected` and `this is an old version of the ship`: the sponsor may have acknowledged messages ahead of the archived local state. See the [Vere 4.6 boot-state check](https://github.com/urbit/vere/blob/vere-v4.6/pkg/vere/pier.c#L527-L561). Preserve that refusal rather than clearing locks, changing keys or weakening the runtime's guard. Prepare any replacement fixture from the latest verified stopped disposable state; do not substitute it for proof of fresh onboarding.

The manually dispatched **Native disposable comet acceptance** workflow runs the same script on four native runners and uploads only `status.json` for seven days. It has no publishing permissions. Prepared workflow configuration is not hosted/native acceptance evidence until an actual run passes.

The [first alpha lifecycle run](https://github.com/davetist/harbor/actions/runs/38387194232)
passed on all four native targets on 13 October 2026. Its downloaded reports
confirmed every lifecycle check and graceful service exit; see
[validation](validation.md#first-named-alpha-release-pass--2026-10-13) for revision and scope.

## Web response recovery

`web/e2e/web-recovery.spec.ts` covers unresponsive Open, explicit confirmation,
cancellation, conflicts, recovery holds, response rechecks, accessibility and
narrow layouts with HTTP fixtures. `web/e2e/web-recovery-native.spec.ts` exercises
the real embedded UI and Vere: a verified private Dojo query runs a long,
allocation-light computation that blocks HTTP; the confirmed interruption
restores it, Open preserves the connected
hostname, and a fresh authenticated session confirms the same identity.

Prepare a new isolated foreground service as described in the agent guide,
using a private temporary directory named `harbor-web-recovery-UNIQUE` with its
registry in `data/`. Boot exactly one disposable comet with folder label
`web-recovery-trial`, wait for its durable job to complete, and supply that
service's loopback URL explicitly:

```sh
HARBOR_WEB_RECOVERY_URL=http://127.0.0.1:ACTUAL_PORT \
  npm --prefix web run test:e2e -- web-recovery-native.spec.ts --workers=1
```

The test checks the private disposable registry marker, comet identity and label,
and canonical pier location before sending any input. A read-only process-record
check verifies that the PID and launch ID stay the same. It disables tracing to
keep login codes and cookies in memory. It does not select an installed service
or an existing operator ship.
Afterward, stop the disposable ship and manager gracefully and confirm that
their writers exited before removing only that trial's temporary files.

## Physical storage trial

The intended physical drive must be attached, mounted and UUID-pinned using [external storage](external-storage.md). Do not use an operator's live pier. Place a new disposable trial in a chosen folder on that drive and record its filesystem, capacity, connection/enclosure and actual allocated bytes.

Verify boot/login, maintenance, export/restore and restart with the drive present. Stop Harbor and verify no writer before a normal unmount. Startup with the drive absent or a replacement UUID must refuse without creating fallback data. Disconnect, enclosure/power-loss and corruption trials require an explicitly disposable drive/partition and agreement on those destructive fault injections; they cannot be inferred from disk-image/mount fixtures. Reattach and inspect recovery holds, archive integrity and actual login before calling recovery accepted.
