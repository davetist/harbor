#!/usr/bin/env python3
"""Run one disposable Vere trial under the PTY profile used in the report."""

import argparse
import errno
import fcntl
import json
import os
import pathlib
import pty
import resource
import select
import signal
import struct
import subprocess
import termios
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", type=pathlib.Path, required=True)
    parser.add_argument("--timeout", type=float, default=60)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command
    if command and command[0] == "--":
        command = command[1:]
    if not command or args.timeout <= 0:
        parser.error("a command and a positive timeout are required")

    # Refuse to replace an existing log. The caller chooses a disposable pier.
    with args.log.open("xb") as log:
        master, slave = pty.openpty()
        proc = None
        try:
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 100, 0, 0))

            def child_setup():
                fcntl.ioctl(0, termios.TIOCSCTTY, 0)
                # A reproduction should not leave a potentially large core file.
                resource.setrlimit(resource.RLIMIT_CORE, (0, 0))

            proc = subprocess.Popen(
                command,
                stdin=slave,
                stdout=slave,
                stderr=slave,
                start_new_session=True,
                preexec_fn=child_setup,
                env=dict(os.environ, TERM="dumb"),
            )
            os.close(slave)
            slave = None
            deadline = time.monotonic() + args.timeout
            timed_out = False
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    timed_out = True
                    break
                readable, _, _ = select.select([master], [], [], min(remaining, 0.2))
                if not readable:
                    continue
                try:
                    data = os.read(master, 8192)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not data:
                    break
                log.write(data)
                log.flush()
            if timed_out:
                stop_group(proc)
            else:
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    timed_out = True
                    stop_group(proc)
            print(json.dumps({"exitCode": proc.returncode, "timedOut": timed_out}))
            return 124 if timed_out else (0 if proc.returncode == 0 else 1)
        finally:
            if proc is not None:
                # This process created the session; signal only that owned group.
                stop_group(proc)
            if slave is not None:
                os.close(slave)
            os.close(master)


def stop_group(proc):
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=3)
    except subprocess.TimeoutExpired:
        pass
    # The session leader can exit before a descendant. Clean up that group too.
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    proc.wait()


if __name__ == "__main__":
    raise SystemExit(main())
