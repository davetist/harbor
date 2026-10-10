"""HTTP contract regressions for the opt-in native acceptance script."""
import contextlib
import http.server
import importlib.util
import io
import json
import pathlib
import platform
import subprocess
import tempfile
import threading
import time
import unittest
import urllib.parse
from unittest import mock

spec = importlib.util.spec_from_file_location("native_acceptance", pathlib.Path(__file__).with_name("native-acceptance.py"))
native = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native)


class LoginHTTPFixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def respond(self, status, body, content_type="application/json", headers=None):
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        for name, value in (headers or {}).items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(json.dumps(body).encode() if content_type == "application/json" else body.encode())

    def do_GET(self):
        state = self.server.fixture
        path = urllib.parse.urlsplit(self.path).path
        if path == "/api/v1/ships":
            self.respond(200, [state["ship"]] if state["onboarded"] else [])
        elif path == "/api/v1/host":
            self.respond(200, {"platform": state["platform"]})
        elif path.startswith("/api/v1/jobs/"):
            kind = path.rsplit("/", 1)[-1]
            self.respond(200, {"kind": kind, "phase": "completed", "shipId": "ship"})
        elif path == "/api/v1/ships/ship":
            self.respond(200, state["ship"])
        elif path == "/":
            self.respond(state["root_status"], "landing", "text/html")
        elif path == "/~/login":
            # Like native Eyre's login page, this includes the public identity.
            self.respond(200, "<html>Login to ~zod</html>", "text/html")
        elif path == "/session.js":
            state["session_requests"] += 1
            if state.get("redirect_session_url"):
                self.respond(303, "", "text/plain", {"Location": state["redirect_session_url"]})
            elif state["redirect_session"] or self.headers.get("Cookie") != "urbauth=fixture":
                self.respond(303, "", "text/html", {"Location": "/~/login"})
            elif state["session_requests"] <= state["pending_requests"]:
                self.respond(404, "pending", "text/html")
            else:
                self.respond(state["session_status"], state["session_body"], state["session_type"])
        else:
            self.respond(404, "missing", "text/plain")

    def do_POST(self):
        state = self.server.fixture
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path == "/~/login":
            state["login_requests"] += 1
            if urllib.parse.parse_qs(body.decode()).get("password") != ["fixture-code"]:
                self.respond(403, "refused", "text/plain")
            else:
                self.respond(200, "logged in", "text/html", {"Set-Cookie": "urbauth=fixture; Path=/"})
        elif self.path == "/api/v1/onboarding/comet":
            state["onboarded"] = True
            self.respond(202, {"jobId": "boot_comet"})
        elif self.path == "/api/v1/ships/ship/access-code":
            self.respond(200, {"code": "fixture-code"})
        elif self.path in ["/api/v1/ships/ship/start", "/api/v1/ships/ship/stop"]:
            kind = self.path.rsplit("/", 1)[-1]
            state["ship"].update(state="running" if kind == "start" else "stopped", desiredRunning=kind == "start")
            self.respond(202, {"jobId": kind})
        else:
            self.respond(404, "missing", "text/plain")


class NativeAcceptanceLoginTests(unittest.TestCase):
    def trial(self, **overrides):
        osname = platform.system().lower()
        arch = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
        if osname == "darwin" and subprocess.run(["sysctl", "-n", "hw.optional.arm64"], capture_output=True, text=True).stdout.strip() == "1":
            arch = "arm64"
        server = http.server.HTTPServer(("127.0.0.1", 0), LoginHTTPFixture)
        state = {"platform": f"{osname}/{arch}", "onboarded": False,
                 "ship": {"state": "running", "identity": "~zod", "webPort": server.server_port, "desiredRunning": True},
                 "root_status": 200, "redirect_session": False, "pending_requests": 0,
                 "session_status": 200, "session_body": "window.ship='zod';", "session_type": "text/javascript",
                 "session_requests": 0, "login_requests": 0}
        state.update(overrides)
        server.fixture = state
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        real_popen = subprocess.Popen

        class TrialManager:
            # Replace only the external manager process. Its HTTP boundary,
            # cookies and all acceptance checks run through real local HTTP.
            def __init__(self, *_args, stdout, **_kwargs):
                self.exit = None
                stdout.write(f"Harbor · http://127.0.0.1:{server.server_port}\n")
                stdout.flush()

            def poll(self):
                return self.exit

            def send_signal(self, _signal):
                self.exit = 0

            def wait(self, **_kwargs):
                return self.exit

        try:
            with tempfile.TemporaryDirectory() as directory:
                root = pathlib.Path(directory)
                binary = root / "fixture-manager"
                binary.write_bytes(b"fixture manager")
                output = root / "trial"
                args = ["native-acceptance.py", "--binary", str(binary), "--output", str(output), "--disposable"]
                # The real readiness deadline is bounded; shorten only this
                # permanent-missing fixture so the regression remains focused.
                timeout = .05 if state["pending_requests"] == 1000000 else 120

                def launch(command, *launch_args, **launch_kwargs):
                    if command[0] == str(binary.resolve()):
                        return TrialManager(command, *launch_args, **launch_kwargs)
                    return real_popen(command, *launch_args, **launch_kwargs)

                with mock.patch.object(native.subprocess, "Popen", launch), mock.patch.object(native, "SESSION_READY_TIMEOUT", timeout, create=True), mock.patch("sys.argv", args), contextlib.redirect_stdout(io.StringIO()):
                    result = native.main()
                report = json.loads((output / "status.json").read_text())
                self.assertEqual(report["checks"].get("onboarding"), "completed", report)
                return result, report, state
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_missing_landing_app_does_not_reject_authenticated_session(self):
        result, report, _ = self.trial(root_status=404)
        self.assertEqual(result, 0, report)
        self.assertTrue(report["checks"]["authenticatedLogin"])

    def test_session_script_waits_for_startup_without_repeating_login(self):
        result, report, state = self.trial(pending_requests=2)
        self.assertEqual(result, 0, report)
        self.assertEqual(state["login_requests"], 2)  # Initial login and the normal restart login.
        self.assertGreater(state["session_requests"], 2)

    def test_login_html_redirect_containing_identity_is_rejected(self):
        result, report, _ = self.trial(redirect_session=True)
        self.assertEqual(result, 1, report)
        self.assertNotIn("authenticatedLogin", report["checks"])

    def test_identity_in_html_at_script_path_is_rejected(self):
        result, report, _ = self.trial(session_type="text/html", session_body="<html>~zod</html>")
        self.assertEqual(result, 1, report)
        self.assertNotIn("authenticatedLogin", report["checks"])

    def test_same_script_path_on_another_origin_is_rejected(self):
        class OtherOrigin(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                self.send_response(200)
                self.send_header("Content-Type", "text/javascript")
                self.end_headers()
                self.wfile.write(b"window.ship='zod';")

        server = http.server.HTTPServer(("127.0.0.1", 0), OtherOrigin)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            result, report, _ = self.trial(redirect_session_url=f"http://127.0.0.1:{server.server_port}/session.js")
            self.assertEqual(result, 1, report)
            self.assertNotIn("authenticatedLogin", report["checks"])
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_wrong_ship_identity_is_rejected(self):
        result, report, _ = self.trial(session_body="window.ship='nec';")
        self.assertEqual(result, 1, report)
        self.assertNotIn("authenticatedLogin", report["checks"])

    def test_expected_identity_in_comment_does_not_replace_ship_assignment(self):
        result, report, _ = self.trial(session_body="// zod\nwindow.ship='nec';")
        self.assertEqual(result, 1, report)
        self.assertNotIn("authenticatedLogin", report["checks"])

    def test_authentication_refusal_is_not_retried(self):
        result, report, state = self.trial(session_status=403)
        self.assertEqual(result, 1, report)
        self.assertEqual(state["session_requests"], 1)

    def test_permanent_script_404_has_a_bounded_deadline(self):
        started = time.monotonic()
        result, report, state = self.trial(pending_requests=1000000)
        self.assertEqual(result, 1, report)
        self.assertGreater(state["session_requests"], 1)
        self.assertLess(time.monotonic() - started, 3)


if __name__ == "__main__":
    unittest.main()
