from __future__ import annotations

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest


SENDER = Path(__file__).with_name("cmcc-notify-send")


class CaptureHandler(BaseHTTPRequestHandler):
    requests: list[dict[str, object]] = []

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", "0"))
        body = json.loads(self.rfile.read(length))
        self.requests.append(
            {
                "path": self.path,
                "authorization": self.headers.get("Authorization"),
                "body": body,
            }
        )
        self.send_response(202)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"status":"accepted"}')

    def log_message(self, format: str, *args: object) -> None:
        return


class SenderTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), CaptureHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        host, port = cls.server.server_address
        cls.url = f"http://{host}:{port}"

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join(timeout=2)

    def setUp(self) -> None:
        CaptureHandler.requests.clear()

    def environment(self) -> dict[str, str]:
        environment = os.environ.copy()
        environment.update(
            {
                "CMCC_NOTIFY_URL": self.url,
                "CMCC_NOTIFY_APPLICATION_TOKEN": "cn_app_test_only",
            }
        )
        return environment

    def run_sender(
        self,
        *arguments: str,
        stdin: dict[str, object] | None = None,
        environment: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SENDER), *arguments],
            input=json.dumps(stdin) if stdin is not None else None,
            text=True,
            capture_output=True,
            env=environment or self.environment(),
            check=False,
        )

    def test_stop_hook_sends_expected_notification_without_output(self) -> None:
        completed = self.run_sender(
            stdin={"hook_event_name": "Stop", "cwd": "/srv/example-project"}
        )

        self.assertEqual(completed.returncode, 0)
        self.assertEqual(completed.stdout, "")
        self.assertEqual(completed.stderr, "")
        self.assertEqual(len(CaptureHandler.requests), 1)
        notification = CaptureHandler.requests[0]
        self.assertEqual(notification["path"], "/v1/notify")
        self.assertEqual(notification["authorization"], "Bearer cn_app_test_only")
        self.assertEqual(
            notification["body"],
            {"title": "Codex 已完成", "message": "example-project\n已完成当前任务"},
        )

    def test_subagent_stop_is_disabled_by_default(self) -> None:
        completed = self.run_sender(
            stdin={"hook_event_name": "SubagentStop", "cwd": "/srv/project"}
        )

        self.assertEqual(completed.returncode, 0)
        self.assertEqual(CaptureHandler.requests, [])

    def test_private_config_can_enable_and_customize_an_event(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "codex.json"
            config.write_text(
                json.dumps(
                    {
                        "url": self.url,
                        "application_token": "cn_app_from_file",
                        "events": {
                            "SubagentStop": {
                                "enabled": True,
                                "title": "代理完成",
                                "message": "{project} / {event}",
                            }
                        },
                    }
                ),
                encoding="utf-8",
            )
            environment = os.environ.copy()
            environment.pop("CMCC_NOTIFY_URL", None)
            environment.pop("CMCC_NOTIFY_APPLICATION_TOKEN", None)
            completed = self.run_sender(
                "--config",
                str(config),
                stdin={"hook_event_name": "SubagentStop", "cwd": "/srv/project"},
                environment=environment,
            )

        self.assertEqual(completed.returncode, 0)
        self.assertEqual(completed.stdout, "")
        self.assertEqual(completed.stderr, "")
        self.assertEqual(len(CaptureHandler.requests), 1)
        self.assertEqual(
            CaptureHandler.requests[0]["authorization"], "Bearer cn_app_from_file"
        )
        self.assertEqual(
            CaptureHandler.requests[0]["body"],
            {"title": "代理完成", "message": "project / SubagentStop"},
        )

    def test_generic_command_mode(self) -> None:
        completed = self.run_sender(
            "--title", "备份完成", "--message", "NAS 备份成功"
        )

        self.assertEqual(completed.returncode, 0)
        self.assertIn("accepted", completed.stdout)
        self.assertEqual(
            CaptureHandler.requests[0]["body"],
            {"title": "备份完成", "message": "NAS 备份成功"},
        )

    def test_hook_failure_is_silent_and_fail_open(self) -> None:
        environment = self.environment()
        environment["CMCC_NOTIFY_URL"] = "http://127.0.0.1:1"
        completed = self.run_sender(
            stdin={"hook_event_name": "Stop", "cwd": "/srv/project"},
            environment=environment,
        )

        self.assertEqual(completed.returncode, 0)
        self.assertEqual(completed.stdout, "")
        self.assertEqual(completed.stderr, "")


if __name__ == "__main__":
    unittest.main()
