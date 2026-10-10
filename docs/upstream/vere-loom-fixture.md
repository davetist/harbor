# Preparing a disposable Vere fixture and matched startup trials

This procedure creates a new fake `~bus` ship. It does not recreate the byte-identical input of the reported crash, and a successful startup does not disprove an intermittent mapping collision. The original frozen input began as a Vere 4.5 fake ship, was upgraded to 4.6, fully replayed, exported and restored before the recorded trials.

The reported input archive has SHA-256 `f07c5aa50dc2a6a34c6da1816dfc187629e0e23ec197479674a07ecea58e1938`, compressed size 367,834,313 bytes and uncompressed file contents of 1,722,211,632 bytes. Its root is `pier/`, its identity is fake `~bus`, and the loaded epoch is `0i154`. It is retained separately and is not included in the support package. The sanitized syscall trace and core analysis are included.

## Prerequisites

For the Linux trace procedure: Python 3, `curl`, `tar`, `sha256sum`, `cp` and `strace`; the official Vere 4.6 binary; and a compatible bootstrap pill for a new ship. The supplied `vere-loom-pty.py` uses only the Python standard library on POSIX systems. It supplies a 24×100 PTY with `TERM=dumb`, disables core dumps, refuses to overwrite a log, and terminates only its own process group on timeout or completion.

Use at least 6 GiB of free storage for the frozen input, two working copies, downloads and logs. Do not run this against a real pier or a pier used by another runner. On a small device, use an already mounted test volume if appropriate; this procedure does not configure storage. `TMPDIR` selects the parent of the temporary directory below. Do not put the copies in a RAM filesystem unless both its capacity and the runtime's RAM needs fit.

## Create a fresh fake input on Linux ARM64

Run from the directory containing the supplied `vere-loom-pty.py`:

```sh
FIXTURE_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/vere-loom-fixture.XXXXXX")"
export FIXTURE_ROOT
mkdir "$FIXTURE_ROOT/runtime"
curl --fail --location --connect-timeout 10 --max-time 1800 \
  https://github.com/urbit/vere/releases/download/vere-v4.6/linux-aarch64.tgz \
  -o "$FIXTURE_ROOT/linux-aarch64.tgz"
sha256sum "$FIXTURE_ROOT/linux-aarch64.tgz"
```

The expected archive SHA-256 is `ca2fa503e542f941021e793190b0498d4fde8d9d8856b266eaebc65e52e2d088`. Compare it before extracting. The extracted binary SHA-256 is `be1957294c565d83af24edae07754f1fbd76e603497c56eb149b98add963b3c8`. For other targets, select the matching asset from the same [Vere 4.6 release](https://github.com/urbit/vere/releases/tag/vere-v4.6) and record its hash; the Linux ARM64 hashes do not apply to them.

```sh
tar -xzf "$FIXTURE_ROOT/linux-aarch64.tgz" -C "$FIXTURE_ROOT/runtime"
VERE_BIN="$FIXTURE_ROOT/runtime/vere"
export VERE_BIN
sha256sum "$VERE_BIN"
"$VERE_BIN" --version
```

Vere 4.6's default bootstrap URL is shown below. A previously obtained compatible pill can be used instead by setting `PILL` to its absolute path. Record that pill's hash: a fresh bootstrap is not byte-identical to the archived reproduction.

```sh
curl --fail --location --connect-timeout 10 --max-time 1800 \
  https://bootstrap.urbit.org/urbit-v4.6.pill \
  -o "$FIXTURE_ROOT/urbit-v4.6.pill"
PILL="$FIXTURE_ROOT/urbit-v4.6.pill"
sha256sum "$PILL"
python3 vere-loom-pty.py --log "$FIXTURE_ROOT/bootstrap.log" --timeout 900 -- \
  "$VERE_BIN" --no-dock --exit --lmdb-map-size 68719476736 \
  --loom 31 --http-ip 127.0.0.1 --http-port 18999 \
  -F bus -B "$PILL" -c "$FIXTURE_ROOT/frozen"
```

Require `exitCode: 0` and `timedOut: false` before proceeding. `-F bus` creates a fake identity and disables Urbit networking; no live keys are needed. The wrapper's core limit can cause a core-limit warning in Vere; it does not turn a failed startup into success. Preserve `bootstrap.log` if boot fails. This bootstrap URL timed out from the preparation host on 10 October 2026; availability was not confirmed. The fresh-boot command was checked against the Vere 4.6 parser and historical fake-boot workflow, rather than presented as a completed fresh bootstrap in this preparation run.

After a successful clean exit, treat `frozen` as immutable. The supplied wrapper was smoke-tested on macOS ARM64 with official Vere 4.6 against a disposable copy of the retained fake input, with fresh lock files and a 64 GiB mapping; that startup and clean exit passed. This validates the wrapper and retained-input procedure, not the fresh bootstrap download or the collision on macOS.

If using the separately supplied original archive instead, verify its archive hash and extract only its `pier/` tree into a new `frozen` directory. Preserve its database and snapshot files. Do not register it in a runner, and do not replace or merge an existing directory. The issue's original lock-file comparison used the archive's lock files; recreating them defines a different trial condition.

## Run one matched pair

Create both working copies from the same stopped `frozen` tree:

```sh
cp -a "$FIXTURE_ROOT/frozen" "$FIXTURE_ROOT/trial-default"
cp -a "$FIXTURE_ROOT/frozen" "$FIXTURE_ROOT/trial-64g"
```

By default both copies preserve the frozen lock files. To compare **fresh lock files**, remove them from both disposable copies, before either launch:

```sh
python3 - <<'PY'
import os
from pathlib import Path
root = Path(os.environ["FIXTURE_ROOT"]).resolve()
assert root.name.startswith("vere-loom-fixture.")
for name in ("trial-default", "trial-64g"):
    for lock in (root / name / ".urb/log").rglob("lock.mdb"):
        lock.unlink()
PY
```

Run the first command, wait for it to finish, then run the second. The second may be run after a nonzero first exit to collect the pair; keep the two exit statuses and error logs distinct.

```sh
python3 vere-loom-pty.py --log "$FIXTURE_ROOT/default.log" --timeout 60 -- \
  strace -f -s 100 -e trace=mmap,munmap,brk,openat,close,fcntl \
  -o "$FIXTURE_ROOT/default.trace" \
  "$VERE_BIN" --no-dock --exit --loom 31 \
  --http-ip 127.0.0.1 --http-port 18999 "$FIXTURE_ROOT/trial-default"

python3 vere-loom-pty.py --log "$FIXTURE_ROOT/64g.log" --timeout 60 -- \
  strace -f -s 100 -e trace=mmap,munmap,brk,openat,close,fcntl \
  -o "$FIXTURE_ROOT/64g.trace" \
  "$VERE_BIN" --no-dock --exit --lmdb-map-size 68719476736 --loom 31 \
  --http-ip 127.0.0.1 --http-port 18999 "$FIXTURE_ROOT/trial-64g"
```

An ASLR collision is not guaranteed. For further trials, replace only the stopped working copies from `frozen`, choose new log/trace names, and keep the lock policy identical across the compared map sizes. Do not reuse already-run database/snapshot state as a control input. Keep at most the frozen tree and the current pair to bound storage usage. Record map size, lock policy, exit status, `MDB_CORRUPTED` and whether an active database mapping actually overlaps a later fixed loom mapping.

Do not disable ASLR or alter kernel mapping limits to force a result. A deterministic runtime regression test is a separate fix-validation task. The historical results were **500 GiB/archived locks: 1 failure in 8**, **500 GiB/fresh locks: 0 in 8**, and **64 GiB/fresh locks: 0 in 16**; those aggregate groups are not a single-variable comparison.

## Preserve evidence and clean up

Keep the selected logs/traces and their hashes. Replace operator paths with placeholders before sharing them. Record `uname -srmo`, OS release, OS page size, runtime version/hash and available `CONFIG_ARM64_VA_BITS` information. The static Linux runtime's musl allocator should not be described using the host's glibc version. Record the pill hash for any fresh fixture.

After all wrappers return and no process uses the fixture, remove only the temporary directory created by this procedure. Preserve a frozen input if further trials are planned. The wrapper creates no binary core file. The prepared support package contains no pier, bootstrap pill, runtime binary or binary core.
