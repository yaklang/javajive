"""Classify connect failures: policy deny vs DNS vs connection-refused."""

from __future__ import annotations

import errno
import socket

POLICY_ERRNOS = {
    errno.EPERM,
    errno.EACCES,
    errno.ENETUNREACH,
    errno.EHOSTUNREACH,
    getattr(errno, "ENONET", 101),
    getattr(errno, "EPROTONOSUPPORT", 93),
    getattr(errno, "ENOPROTOOPT", 92),
    getattr(errno, "EAFNOSUPPORT", 97),
    getattr(errno, "EHOSTDOWN", 112),
}
REFUSED_ERRNOS = {errno.ECONNREFUSED}


def routable_ipv4() -> str:
    """Local address the host would use for egress. Numeric; no DNS in the worker."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        sock.connect(("192.0.2.1", 80))
        ip = sock.getsockname()[0]
    finally:
        sock.close()
    return ip


def classify_oserror(exc: BaseException) -> str:
    if isinstance(exc, socket.gaierror):
        return "dns_failure"
    if isinstance(exc, socket.timeout):
        # Blackhole drop is a policy outcome when isolation is active; still not DNS.
        return "policy_deny"
    en = getattr(exc, "errno", None)
    if en in REFUSED_ERRNOS:
        return "connect_refused"
    if en in POLICY_ERRNOS:
        return "policy_deny"
    text = str(exc).lower()
    return classify_text(text)


def classify_text(text: str) -> str:
    t = text.lower()
    if "network_class=connected" in t or t.strip() == "connected":
        return "connected"
    if "network_class=policy_deny" in t:
        return "policy_deny"
    if "network_class=connect_refused" in t:
        return "connect_refused"
    if "network_class=dns_failure" in t:
        return "dns_failure"
    if any(
        s in t
        for s in (
            "name or service not known",
            "nodename nor servname",
            "temporary failure in name resolution",
            "getaddrinfo",
            "nxdomain",
            "unknown host",
            "not known",
            "bad address",
            "nodename nor servname provided",
        )
    ):
        return "dns_failure"
    if "connection refused" in t or "econnrefused" in t:
        return "connect_refused"
    if any(
        s in t
        for s in (
            "network is unreachable",
            "network unreachable",
            "operation not permitted",
            "permission denied",
            "no route to host",
            "host is unreachable",
            "protocol not available",
            "address family not supported",
            "network down",
            "sandbox",
            "blocked",
        )
    ):
        return "policy_deny"
    return "other"


PROBE_PYTHON = r'''
import errno, socket, sys
ip, port = sys.argv[1], int(sys.argv[2])
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(3)
try:
    s.connect((ip, port))
    data = s.recv(64)
    sys.stdout.write("NETWORK_CLASS=connected\n")
    sys.stdout.write(repr(data) + "\n")
    sys.exit(10)
except socket.gaierror as e:
    sys.stdout.write("NETWORK_CLASS=dns_failure\n")
    sys.stdout.write(repr(e) + "\n")
    sys.exit(2)
except Exception as e:
    en = getattr(e, "errno", None)
    refused = {errno.ECONNREFUSED}
    policy = {errno.EPERM, errno.EACCES, errno.ENETUNREACH, errno.EHOSTUNREACH}
    if isinstance(e, socket.timeout) or en in policy:
        cls = "policy_deny"
        code = 3
    elif en in refused:
        cls = "connect_refused"
        code = 4
    else:
        cls = "other"
        code = 5
    sys.stdout.write("NETWORK_CLASS=%s\n" % cls)
    sys.stdout.write("errno=%s %r\n" % (en, e))
    sys.exit(code)
'''

PROBE_SH = r'''
#!/bin/sh
IP="$1"
PORT="$2"
ERR=$(wget -T 3 -O /tmp/t30net.out "http://${IP}:${PORT}/" 2>&1)
EC=$?
echo "$ERR"
if echo "$ERR" | grep -qiE 'network unreachable|network is unreachable|operation not permitted|permission denied|no route to host|host is unreachable'; then
  echo NETWORK_CLASS=policy_deny
  exit 3
fi
if echo "$ERR" | grep -qiE 'connection refused'; then
  echo NETWORK_CLASS=connect_refused
  exit 4
fi
if echo "$ERR" | grep -qiE 'bad address|name or service|not known|resolve|unknown host'; then
  echo NETWORK_CLASS=dns_failure
  exit 2
fi
if [ "$EC" -eq 0 ]; then
  echo NETWORK_CLASS=connected
  cat /tmp/t30net.out 2>/dev/null
  exit 10
fi
echo NETWORK_CLASS=other
exit 5
'''
