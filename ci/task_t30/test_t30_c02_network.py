"""T30-C02: worker cannot reach a live host TCP server; failure is policy, not DNS or forgotten server."""

from __future__ import annotations

import socket
import socketserver
import threading
import unittest

from harness import obs_dict, worker, write_evidence
from tools.sandbox_worker.network import routable_ipv4


class _Handler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        try:
            self.request.sendall(b"T30-C02-ALIVE\n")
        except OSError:
            pass


class TestTaskT30C02Network(unittest.TestCase):
    def test_T30_C02_policy_blocks_live_local_tcp(self) -> None:
        w = worker()
        server = socketserver.ThreadingTCPServer(("0.0.0.0", 0), _Handler)
        server.allow_reuse_address = True
        port = server.server_address[1]
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            host_ip = routable_ipv4()
            # Control: the server is actually up. A forgotten server would fail here.
            with socket.create_connection(("127.0.0.1", port), timeout=2) as sock:
                banner = sock.recv(32)
            self.assertIn(b"T30-C02-ALIVE", banner)
            host_ip_reachable = False
            try:
                with socket.create_connection((host_ip, port), timeout=1) as sock:
                    banner2 = sock.recv(32)
                host_ip_reachable = b"T30-C02-ALIVE" in banner2
            except OSError:
                host_ip_reachable = False

            if w.backend is None:
                obs = w.probe_network(host_ip, port)
                self.assertEqual(obs.reason, "isolation_unavailable")
                self.assertFalse(obs.did_execute)
                write_evidence(
                    "T30-C02",
                    {
                        "backend": None,
                        "fail_closed": True,
                        "server_alive": True,
                        "target": f"{host_ip}:{port}",
                        "observation": obs_dict(obs),
                    },
                )
                return

            obs = w.probe_network(host_ip, port)
            text = obs.stdout + "\n" + obs.stderr
            self.assertTrue(obs.did_execute)
            self.assertIsNotNone(obs.policy_digest)
            self.assertNotIn("T30-C02-ALIVE", text)
            self.assertNotEqual(obs.network_class, "connected")
            self.assertNotEqual(obs.network_class, "dns_failure")
            # Numeric IP: DNS must not be the failure mode.
            self.assertNotIn("name or service not known", text.lower())
            self.assertEqual(obs.network_class, "policy_deny")
            self.assertNotEqual(obs.network_class, "connect_refused")
            self.assertEqual((obs.policy or {}).get("network"), "none")
            self.assertTrue(
                "network unreachable" in text.lower() or "operation not permitted" in text.lower(),
                text,
            )
            write_evidence(
                "T30-C02",
                {
                    "backend": w.backend,
                    "policy_digest": obs.policy_digest,
                    "server_alive": True,
                    "host_control_loopback": True,
                    "host_control_routable": host_ip_reachable,
                    "target": f"{host_ip}:{port}",
                    "network_class": obs.network_class,
                    "observation": obs_dict(obs),
                },
            )
        finally:
            server.shutdown()
            server.server_close()


if __name__ == "__main__":
    unittest.main()
