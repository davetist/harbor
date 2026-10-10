# Fixed loom mappings can overwrite live LMDB mappings on POSIX

## Summary

Vere's POSIX loom initialization uses `MAP_FIXED` without first reserving the required address ranges. If LMDB or another allocation occupies one of those ranges, mapping the loom can replace memory that the process still uses.

This was reproduced with the official Vere 4.6 binary in a standalone fake-ship fixture on Linux ARM64: the loom replaced part of the live LMDB mapping and an adjacent allocator mapping, causing `MDB_CORRUPTED` followed by a fault during database cleanup.

**The source-level concern is not specific to ARM64.** Linux x86_64 and macOS use the same unprotected mapping path in the reviewed upstream source. The failure has not been reproduced on those platforms; their address-space layouts may make it less likely or prevent this particular layout.

## Platform and version scope

| Platform | Evidence |
| --- | --- |
| Linux ARM64 | Runtime failure reproduced and traced on Vere 4.6. |
| Linux x86_64 | Shares the unprotected POSIX source path; runtime failure not reproduced. |
| macOS | Shares the unprotected POSIX source path; runtime failure not reproduced. |
| Windows | The reviewed upstream snapshot has a separate address reservation implementation. This report does not establish an equivalent Windows failure. |

The reproduction is on released **Vere 4.6**. Source review is against **`5faf531e26512585c4c0004598dfa7005278e1cf`**, inspected on 10 October 2026. That `develop` snapshot was not built or tested for this report.

At that snapshot, [`u3_disk_stake()`](https://github.com/urbit/vere/blob/5faf531e26512585c4c0004598dfa7005278e1cf/pkg/vere/disk.c#L1630-L1673) reserves native and migration loom addresses only on Windows. The [worker calls it before loading the pier](https://github.com/urbit/vere/blob/5faf531e26512585c4c0004598dfa7005278e1cf/pkg/vere/main.c#L3140-L3152), but it is a no-op on POSIX. [`u3m_init()` subsequently uses `MAP_FIXED`](https://github.com/urbit/vere/blob/5faf531e26512585c4c0004598dfa7005278e1cf/pkg/noun/manage.c#L2633-L2640), which replaces overlapping mappings. This replacement behavior is specified by [POSIX mmap](https://pubs.opengroup.org/onlinepubs/9799919799/functions/mmap.html).

## Expected behavior

A fixed loom mapping should replace only a reservation or loom mapping that Vere already owns for that purpose. If the required range is occupied, or the requested LMDB map cannot fit after reservation, startup should fail with a clear allocation error before replacing unrelated memory.

## Reproduction environment and results

- Raspberry Pi 4, 8 GB RAM; Linux AArch64, kernel `6.18.50+rpt-rpi-v8`, Debian 13 (trixie), 4 KiB OS pages and `CONFIG_ARM64_VA_BITS=39`. Kernel/OS configuration was read after the trials on the same boot, which began on 7 October 2026.
- Official Vere 4.6 `linux-aarch64` binary; musl mallocng allocator identified in the core. Exact musl version was not recorded; the host's glibc version is not the static runtime's libc version.
- Frozen fake `~bus` pier; epoch `0i154`, 4 KiB LMDB pages, 762,478,592-byte snapshot.
- `--loom 31` throughout: a 2 GiB loom.
- Direct runtime launch under a 24×100 PTY with `TERM=dumb`.

Binary SHA-256:

```text
be1957294c565d83af24edae07754f1fbd76e603497c56eb149b98add963b3c8
```

Before every trial, the harness restored identical archived database and snapshot bytes. Lock-file handling differed between groups:

| LMDB map size | Lock files | Starts | Failed starts | Traces with the observed live overlap |
| --- | --- | ---: | ---: | ---: |
| Default, 500 GiB | Archived | 8 | 1 | 1 |
| Default, 500 GiB | Recreated | 8 | 0 | 0 |
| Explicit, 64 GiB | Recreated | 16 | 0 | 0 |

**This is not a matched single-variable comparison.** All 64 GiB trials recreated the lock files; the failing 500 GiB trial used archived locks. The 500 GiB fresh-lock group also passed 8/8. These counts do not isolate the effect of map size. The failed worker's syscall trace and core independently establish that its fixed loom mapping replaced live database and allocator mappings.

The command below uses `$VERE_BIN` for that runtime and `$TEST_PIER` for a disposable copy of the frozen fake pier:

```sh
strace -f -s 100 -e trace=mmap,munmap,brk,openat,close,fcntl \
  -o trace.log "$VERE_BIN" --no-dock --exit --loom 31 \
  --http-ip 127.0.0.1 --http-port 18999 "$TEST_PIER"
```

The 64 GiB series added `--lmdb-map-size 68719476736` before the pier argument and recreated every `lock.mdb` before each start. This is an intermittent reproduction: the command alone does not guarantee a collision. The fixture preparation procedure and a complete same-PID trace excerpt are included below. The support files contain the full sanitized syscall trace, selected core analysis and a PTY wrapper. The original 367,834,313-byte fake-pier archive is retained separately; it is not included in that small support package.

## Trace and core evidence

The failed worker PID `23748` opened the epoch database, mapped it, made an adjacent anonymous allocation, and then mapped the loom. Paths are sanitized; descriptors, addresses, sizes and order are unchanged:

```text
23748 openat(AT_FDCWD, "<fake-pier>/.urb/log/0i154/data.mdb", O_RDWR|O_CREAT|O_LARGEFILE, 0664) = 16
23748 mmap(NULL, 536870912000, PROT_READ, MAP_SHARED, 16, 0) = 0x27bc2f000
23748 openat(AT_FDCWD, "<fake-pier>/.urb/log/0i154/data.mdb", O_WRONLY|O_DSYNC|O_LARGEFILE|O_CLOEXEC) = 17
23748 fcntl(17, F_SETFD, FD_CLOEXEC) = 0
23748 fcntl(15, F_SETLK, {l_type=F_RDLCK, l_whence=SEEK_SET, l_start=0, l_len=1}) = 0
23748 mmap(NULL, 16384, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0) = 0x27bc2b000
23748 mmap(0x200000000, 2147483648, PROT_READ|PROT_WRITE, MAP_PRIVATE|MAP_FIXED|MAP_ANONYMOUS, -1, 0) = 0x200000000
```

There is no intervening unmap of those epoch/allocator mappings. The full trace also retains the earlier top-level database open and unmap, so it can be distinguished from the later live epoch mapping at the same address.

The loom occupies `[0x200000000, 0x280000000)`. Its mapping replaces **71,110,656 bytes** of the live database mapping, plus the 16 KiB anonymous allocation immediately below it. Metadata lookup then reports:

```text
lmdb: meta read: dbi_open fail: MDB_CORRUPTED: Located page was wrong type
disk: read meta: no identity
mars: disk meta fail
loom: external fault: 0x10 (0x200000000 : 0x280000000)
```

The core shows `mdb_env_close0` calling `__libc_free(0x27bc2b0b0)`, followed by a fault in musl's `get_meta`. The database mapping's surviving portion starts at `0x280000000` with file offset `0x43d1000`, confirming that its lower portion was replaced.

All three databases in the frozen input archive were independently scanned with the same pinned LMDB library, through MAIN/META and EVENTS where present. Those reads passed and the input database hashes remained unchanged. The traced failure is process-memory replacement; this does not establish persistent disk corruption or attribute every `MDB_CORRUPTED` report to the same cause.

## Existing workaround

All 16 observed starts using `--lmdb-map-size 68719476736` with fresh lock files completed without the collision. The default-size fresh-lock group also passed; this is encouraging mitigation evidence, not proof that map-size reduction alone changed the outcome. The option limits LMDB's virtual mapping to 64 GiB; it does not allocate 64 GiB of physical RAM.

The option changes the mapping size without enforcing address ownership. These controls support it as a workaround for the tested fixture, not a universal guarantee or default. A smaller limit must accommodate the database and growth headroom. Reducing the default alone would leave the unsafe fixed-mapping behavior in place.

Harbor subsequently implemented adaptive startup mapping with documented retirement criteria. It selects at least twice the largest database's logical file size, rounded up to a power of two with a 64 GiB minimum, on every ship launch across its macOS/Linux ARM64 and x86_64 targets. This is a downstream capacity policy derived from the 64 GiB controls; it does not implement the address-ownership fix proposed below. Larger selected maps can still encounter overlap or address-space exhaustion. The override covers ship restarts after utilities, not the utility processes themselves; Vere 4.6's `play` and `prep` parsers do not accept it. Subsequent native Harbor startup checks are recorded separately and do not change this report's original 16-trial series or establish equivalent collision reproduction on other targets.

## Proposed fix

Extend the existing address reservation approach to the POSIX path:

1. Determine the actual snapshot/version and migration needs before LMDB opens. Reserve the native range and only migration ranges this invocation will use, or establish a different mapping policy that is compatible with the available address space. Do not copy Windows' unconditional reservation of all possible stale-loom bases onto constrained POSIX hosts. Reserve in each exec-created worker; parent reservations do not survive exec.
2. Acquire ranges without replacing existing mappings. Linux can use `MAP_FIXED_NOREPLACE`, checking the returned address for older-kernel compatibility. A portable alternative is a non-fixed `mmap` hint, accepted only when the exact address is returned; otherwise release only the newly created mapping and fail. Never force success by falling back to an unreserved `MAP_FIXED`. See [Linux mmap(2)](https://www.man7.org/linux/man-pages/man2/mmap.2.html).
3. Track ownership and allow fixed replacement only wholly within an owned range. Preserve legitimate image remapping, demand paging and supported ephemeral backing.
4. Reuse [`_disk_migrate_size()` and existing migration bounds](https://github.com/urbit/vere/blob/5faf531e26512585c4c0004598dfa7005278e1cf/pkg/vere/disk.c#L1599-L1628). Handle lite/full startup, repeated initialization and differing claim lengths. Acquire additional addresses safely when growing a range; do not release and reacquire an existing reservation through an unprotected gap.
5. Preserve `--lmdb-map-size` and worker propagation. Report reservation conflicts or insufficient virtual address space clearly. Reservation cannot guarantee that a contiguous 500 GiB database map will fit on every host.

In the reviewed ordinary Linux build, the alternate noun-width loom begins at 136 GiB: the native base is 8 GiB, and the alternate offset is 128 GiB. Every recorded 500 GiB database mapping spans that address. On the tested 39-bit host, reserving even a small range there splits the 512 GiB virtual address space so that no contiguous 500 GiB interval remains. Unnecessary migration reservations would therefore make a non-migrating default-size startup fail. This is why range selection and ordinary startup coverage are part of the fix, rather than treating the Windows policy as a drop-in solution.

This proposes a POSIX address-ownership invariant, with platform-specific reservation primitives and native validation. It is not a blanket replacement of every `MAP_FIXED` call with `MAP_FIXED_NOREPLACE`. Preserve the existing Windows behavior and tests. No patched runtime has been validated yet.

## Regression coverage

- A deterministic occupied-address test: place a file-backed sentinel across a requested loom range in a disposable subprocess. Reservation must fail without replacing the sentinel or changing its contents. The test must expose the old unsafe behavior.
- Test ordinary non-migrating startup on constrained address space: it must not acquire unnecessary alternate-loom reservations. Then reserve and open LMDB; database mappings must avoid owned loom ranges, or fail cleanly if no suitable range fits.
- Exercise normal and `--no-demand` image loading, repeated reservations, smaller claims and conflicting expansion.
- Exercise supported version and noun-width migrations with separate source/destination reservations and correct release. CPU architecture and `VERE64` noun width are separate test dimensions.
- Cover parent lite startup, exec-created worker startup, replay and maintenance commands that map a loom/database; verify explicit map-size propagation.
- Force address-space exhaustion and require a clear error without an allocator crash or destructive remapping.
- Run native checks on Linux ARM64, Linux x86_64 and macOS before claiming coverage for those platforms; preserve Windows regression coverage. Repeat the frozen fixture trials and inspect mappings as well as exit status.

Deterministic tests should be the primary regression gate. Successful repeated starts under ASLR supplement them. Use small disposable mapping tests where possible, and bounded fake-pier trials with cleanup of their owned fixtures and cores.

## Related issues and prior work

The following references were researched on 10 October 2026. Similar symptoms do not establish an identical root cause, and this report does not claim to resolve them.

| Reference | Relevance |
| --- | --- |
| [Vere #620](https://github.com/urbit/vere/issues/620) | Closest crash signature: Pi startup fault at `0x10`, with `get_meta` and `__libc_free` frames. Its initial cause was not established; the new trace provides a mechanism to compare. |
| [Vere PR #1081](https://github.com/urbit/vere/pull/1081) | Existing Windows implementation precedent: native and migration address reservations before LMDB, with address-space tests. |
| [Urbit #6947](https://github.com/urbit/urbit/issues/6947) | Oversized LMDB mapping prevents Pi startup with `ENOMEM`. Related address-space pressure, a different observed failure from replacement of a live mapping. |
| [Vere PR #655](https://github.com/urbit/vere/pull/655) | Introduced the `--lmdb-map-size` option used in these controls. |
| [Vere #878](https://github.com/urbit/vere/issues/878), [PR #879](https://github.com/urbit/vere/pull/879), [PR #873](https://github.com/urbit/vere/pull/873) | Separate map-size propagation bug. The fix from #879 landed through #873, as explained in the [maintainer comment](https://github.com/urbit/vere/pull/879#issuecomment-3329696544). Preserve forwarding in regression coverage. |
| [Vere #1086](https://github.com/urbit/vere/issues/1086), [PR #1087](https://github.com/urbit/vere/pull/1087) | Separate source/destination loom overlap during 32-to-64 noun migration. Reuse the resulting bounds when reserving migration ranges. |
| [Vere #714](https://github.com/urbit/vere/issues/714) | `MDB_CORRUPTED` on macOS ARM64. No mapping trace establishes the same cause; this is a diagnostic comparison, not confirmation of macOS reproduction. |
| [Vere #253](https://github.com/urbit/vere/issues/253) | Pi Chop assertions/allocation failures; later discussion links startup symptoms in #620. Root-cause equivalence remains unconfirmed. |

Other examined reports concern distinct mechanisms: [#624](https://github.com/urbit/vere/issues/624), `mprotect`/mapping-count exhaustion; [#451](https://github.com/urbit/vere/issues/451), snapshot patching after a loom-size change and unclean shutdown; and [#233](https://github.com/urbit/vere/issues/233), an older ARM64 crash and snapshot blit. They are included to distinguish the scope, not as additional reproductions of this collision.
