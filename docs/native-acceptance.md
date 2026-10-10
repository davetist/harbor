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

Failures retain all trial state. Graceful stop is bounded and does not force-kill a ship. Inspect `status.json`, job history and private output; never overwrite/delete a failed pier to make acceptance pass. Check for surviving writers before reusing a trial. First comet bootstrap can take longer than its later restarts; a successful login does not certify every early shutdown window.

The manually dispatched **Native disposable comet acceptance** workflow runs the same script on four native runners and uploads only `status.json` for seven days. It has no publishing permissions. Prepared workflow configuration is not hosted/native acceptance evidence until an actual run passes.

## Physical storage trial

The intended physical drive must be attached, mounted and UUID-pinned using [external storage](external-storage.md). Do not use an operator's live pier. Place a new disposable trial under a permitted folder on that drive and record its filesystem, capacity, connection/enclosure and actual allocated bytes.

Verify boot/login, maintenance, export/restore and restart with the drive present. Stop Harbor and verify no writer before a normal unmount. Startup with the drive absent or a replacement UUID must refuse without creating fallback data. Disconnect, enclosure/power-loss and corruption trials require an explicitly disposable drive/partition and agreement on those destructive fault injections; they cannot be inferred from disk-image/mount fixtures. Reattach and inspect recovery holds, archive integrity and actual login before calling recovery accepted.
