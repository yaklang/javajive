"""T30 isolation policy constants. Timeout-only subprocess is not a backend."""

from __future__ import annotations

POLICY_VERSION = "t30-isolation-policy-v1"
POLICY_LOCK = "execution-sandbox-policy"

# Multi-arch index digest from `docker buildx imagetools inspect alpine:3.20`.
ALPINE_IMAGE = (
    "alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc"
)
TOOLCHAIN_IMAGE = "javajive-sandbox-toolchain:t30"

SANDBOX_USER = "65534:65534"
SANDBOX_USER_UID = 65534
SANDBOX_USER_GID = 65534

CONTAINER_INPUTS = "/inputs"
CONTAINER_ARTIFACTS = "/artifacts"
CONTAINER_WORK = "/work"
CONTAINER_TMP = "/tmp"
CONTAINER_HOME = "/tmp/sandbox-home"
CONTAINER_JDK = "/toolchain/jdk"

REASON_ISOLATION_UNAVAILABLE = "isolation_unavailable"
REASON_CAPABILITY_UNSUPPORTED = "capability_unsupported"
REASON_NETWORK_FORBIDDEN = "network_forbidden"
REASON_PATH_ESCAPE = "artifact_escape"
REASON_RESOURCE_LIMIT = "resource_limit"
REASON_TIMEOUT = "timeout"
REASON_POLICY_DENY = "policy_deny"

STATUS_OK = "ok"
STATUS_BLOCKED = "blocked"
STATUS_INVALID_INPUT = "invalid_input"
STATUS_UNSUPPORTED = "unsupported"
STATUS_INFRA_ERROR = "infra_error"
STATUS_TIMEOUT = "timeout"
STATUS_RESOURCE = "resource_limit"

LABEL_JOB = "javajive.t30"
LABEL_ID = "javajive.t30.job"

JAVAC_SAFE_FLAGS = ("-proc:none", "-encoding", "UTF-8")

FAKE_CANARY_VALUE = "T30-FAKE-TOKEN-NOT-A-SECRET"

DOCKER_SOCK_PATHS = (
    "/var/run/docker.sock",
    "/run/docker.sock",
    "/var/run/podman/podman.sock",
    "/run/podman/podman.sock",
)

TOKEN_BASENAMES = (
    ".netrc",
    ".git-credentials",
    "id_rsa",
    "id_ed25519",
    "github_token",
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "credentials.json",
    ".npmrc",
    ".pypirc",
    "config.json",
)

DEFAULT_PATH = "/usr/lib/jvm/java-21-openjdk/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
