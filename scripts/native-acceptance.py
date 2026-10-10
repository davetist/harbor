#!/usr/bin/env python3
"""Opt-in native onboarding/restart/export/restore trial with private evidence.

This never selects an existing manager or pier. Supply only a disposable key
when testing keyfile mode. All trial state is preserved, including failures.
"""
import argparse
import hashlib
import http.client
import http.cookiejar
import json
import pathlib
import platform
import signal
import subprocess
import time
import urllib.parse
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--disposable", action="store_true", required=True)
    parser.add_argument("--require-platform", choices=["darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"])
    parser.add_argument("--mode", choices=["comet", "keyfile"], default="comet")
    parser.add_argument("--key", type=pathlib.Path)
    parser.add_argument("--identity")
    parser.add_argument("--lifecycle", action="store_true", help="also Pack/Roll/Chop/export/restore/replay; not a version upgrade or power-loss test")
    args = parser.parse_args()
    arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    osname = platform.system().lower()
    if osname == "darwin":
        silicon = subprocess.run(["sysctl", "-n", "hw.optional.arm64"], capture_output=True, text=True)
        if silicon.stdout.strip() == "1":
            arch = "arm64"  # Rosetta is not native Intel acceptance.
    native = f"{osname}/{arch}"
    if args.require_platform and args.require_platform != native:
        parser.error(f"This host is {native}; cannot claim {args.require_platform} acceptance")
    if args.mode == "keyfile" and (not args.key or not args.identity):
        parser.error("keyfile mode needs a private disposable key path and its identity")
    binary = args.binary.resolve(strict=True)
    work = args.output.absolute()
    work.mkdir(mode=0o700, parents=True, exist_ok=False)
    config = work / "config.json"
    config.write_text(json.dumps({"listenAddress": "127.0.0.1:0", "defaultShipHTTPBindAddress": "127.0.0.1", "dataDir": str(work / "data")}))
    report = {"platform": native, "mode": args.mode, "binarySHA256": hashlib.sha256(binary.read_bytes()).hexdigest(), "checks": {}, "complete": False}
    process = None
    owned_ships = []
    cookies = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(cookies))
    url = ""

    def request(path, body=None):
        req = urllib.request.Request(url + "/api/v1/" + path, data=json.dumps(body).encode() if body is not None else None, headers={"Content-Type": "application/json"})
        with opener.open(req, timeout=60) as response:
            return json.load(response) if response.status != 204 else None

    def upload(path, kind):
        # Stream archives rather than copying a pier-sized upload into RAM.
        boundary = uuid.uuid4().hex
        prefix = f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="input"\r\nContent-Type: application/octet-stream\r\n\r\n'.encode()
        suffix = f"\r\n--{boundary}--\r\n".encode()
        address = urllib.parse.urlsplit(url)
        connection = http.client.HTTPConnection(address.hostname, address.port, timeout=120)
        try:
            connection.putrequest("POST", "/api/v1/uploads/" + kind)
            connection.putheader("Content-Type", "multipart/form-data; boundary=" + boundary)
            connection.putheader("Content-Length", str(len(prefix) + path.stat().st_size + len(suffix)))
            connection.endheaders(); connection.send(prefix)
            with path.open("rb") as source:
                while chunk := source.read(1 << 20):
                    connection.send(chunk)
            connection.send(suffix)
            response = connection.getresponse()
            if response.status != 200:
                raise RuntimeError("upload rejected")
            return json.load(response)["uploadId"]
        finally:
            connection.close()

    def wait(jobid):
        deadline = time.monotonic() + 1250
        while time.monotonic() < deadline:
            job = request("jobs/" + jobid)
            if job["phase"] in ["completed", "failed", "skipped", "interrupted"]:
                if job["phase"] != "completed":
                    raise RuntimeError(f"{job['kind']} {job['phase']}; inspect private retained output")
                return job
            time.sleep(1)
        raise RuntimeError("job timed out; state preserved")

    def action(ship, kind):
        result = wait(request(f"ships/{ship}/{kind}", {})["jobId"])
        report["checks"][kind] = result["phase"]
        return result

    def login(shipid):
        ship = request("ships/" + shipid)
        assert ship["state"] == "running" and ship.get("identity")
        code = request("ships/" + shipid + "/access-code", {})["code"]
        destination = f"http://127.0.0.1:{ship['webPort']}"
        cookies.clear()  # Prove this login, without reusing an earlier session.
        req = urllib.request.Request(destination + "/~/login", data=urllib.parse.urlencode({"password": code}).encode())
        with opener.open(req, timeout=60) as response:
            assert response.status == 200
        del code
        assert list(cookies), "login did not issue a session cookie"
        with opener.open(destination + "/", timeout=60) as response:
            assert "/~/login" not in urllib.parse.urlsplit(response.geturl()).path
        with opener.open(destination + "/session.js", timeout=60) as response:
            session = response.read(65536)
            assert ship["identity"].lstrip("~").encode() in session
        report["checks"]["authenticatedLogin"] = True

    try:
        with (work / "service.log").open("w+") as log:
            process = subprocess.Popen([str(binary), "serve", "--config", str(config)], stdout=log, stderr=subprocess.STDOUT)
            for _ in range(150):
                text = (work / "service.log").read_text()
                lines = [line for line in text.splitlines() if " · http://" in line]
                if lines:
                    url = lines[-1].split(" · ")[-1]
                    break
                if process.poll() is not None:
                    raise RuntimeError("manager exited before listening")
                time.sleep(.1)
            assert url and request("ships") == []
            assert request("host")["platform"] == native
            report["checks"]["ephemeralListener"] = True
            body = {"folderName": "trial-comet", "loomExponent": 31}
            if args.mode == "keyfile":
                body.update(name=args.identity, uploadId=upload(args.key, "keyfile"))
            job = wait(request("onboarding/" + args.mode, body)["jobId"])
            ship = job["shipId"]; owned_ships.append(ship)
            report["checks"]["onboarding"] = "completed"
            if args.mode == "keyfile":
                assert not list((work / "data/uploads").glob("*.key"))
                report["checks"]["stagedKeyRemoved"] = True
            login(ship)
            action(ship, "stop"); action(ship, "start"); login(ship)
            if args.lifecycle:
                for operation in ["pack", "roll", "chop"]:
                    action(ship, operation)
                action(ship, "backup")
                assert request("ships/" + ship)["desiredRunning"] is False
                backup = request("ships/" + ship + "/backups")[0]
                archive = work / "export.tar.gz"
                with opener.open(url + "/api/v1/backups/" + backup["id"] + "/download", timeout=300) as response, archive.open("xb") as output:
                    while chunk := response.read(1 << 20): output.write(chunk)
                restored = wait(request("onboarding/existing", {"importArchive": True, "uploadId": upload(archive, "archive")})["jobId"])["shipId"]
                owned_ships.append(restored)
                login(restored)
                report["checks"]["exportRestore"] = True
                # The import's source archive is not a replay checkpoint for
                # this newly registered ship. Create its own verified export.
                action(restored, "backup"); action(restored, "start")
                action(restored, "replay"); login(restored)
                report["checks"]["exportRestoreReplay"] = True
            for ship in owned_ships:
                action(ship, "stop")
                assert request("ships/" + ship)["desiredRunning"] is False
            report["complete"] = True
    except Exception as error:
        # Do not print exception bodies/tracebacks that may contain credentials.
        report["failureType"] = type(error).__name__
    finally:
        if process is not None and process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try: process.wait(timeout=90)
            except subprocess.TimeoutExpired: report["shutdownTimedOut"] = True
        report["serviceExit"] = process.poll() if process else None
        report["complete"] = report["complete"] and report["serviceExit"] == 0
        (work / "status.json").write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report, indent=2))
    return 0 if report["complete"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
