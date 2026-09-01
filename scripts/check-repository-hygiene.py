#!/usr/bin/env python3
"""Reject sensitive local state and high-confidence credentials tracked by Git."""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import PurePosixPath


FORBIDDEN_NAMES = {
    ".env", ".git-credentials", ".netrc", ".npmrc", ".pypirc", "auth.json",
    "credentials.json", "kubeconfig",
}
FORBIDDEN_PARTS = {
    ".aws", ".azure", ".kube", ".thinkpixelws", "browser-profiles",
    "chrome-profile", "chromium-profile", "firefox-profile", "local-data",
    "portable-snapshots", "user data", "workspace-snapshots",
}
FORBIDDEN_SUFFIXES = {
    ".credentials", ".jks", ".key", ".keystore", ".kubeconfig", ".p12",
    ".pem", ".pfx", ".token",
}

# Assemble marker strings so the policy source does not match itself.
SECRET_PATTERNS = {
    "private key": re.compile(b"-----BEGIN " + b"(?:[A-Z0-9]+ )?PRIVATE KEY-----"),
    "GitHub token": re.compile(b"gh" + b"[pousr]_[A-Za-z0-9]{36,255}"),
    "AWS access key": re.compile(b"(?:AKIA|ASIA)[0-9A-Z]{16}"),
    "Google service-account key": re.compile(
        b'"type"\\s*:\\s*"service_account"[\\s\\S]{0,4096}"private_key"\\s*:'
    ),
    "Slack token": re.compile(b"xox" + b"[aboprs]-[A-Za-z0-9-]{10,}"),
}


def path_reason(name: str) -> str | None:
    path = PurePosixPath(name)
    lowered_parts = tuple(part.lower() for part in path.parts)
    basename = lowered_parts[-1]
    if basename.startswith(".env.") and basename not in {".env.example", ".env.sample"}:
        return "environment file"
    if basename in {"id_dsa", "id_ecdsa", "id_ed25519", "id_rsa"}:
        return "private SSH key path"
    if basename in FORBIDDEN_NAMES:
        return "credential or local configuration path"
    if any(part in FORBIDDEN_PARTS for part in lowered_parts):
        return "Workspace, profile, credential, or local-data path"
    if any(basename.endswith(suffix) for suffix in FORBIDDEN_SUFFIXES):
        return "token, key, credential, or kubeconfig file"
    if lowered_parts[:2] == (".docker", "config.json"):
        return "container registry credential file"
    if lowered_parts[:3] == (".config", "gcloud", "credentials.db"):
        return "cloud credential file"
    return None


def findings(files: list[tuple[str, bytes]]) -> list[str]:
    result: list[str] = []
    for name, content in files:
        reason = path_reason(name)
        if reason:
            result.append(f"{name}: forbidden {reason}")
            continue
        if b"\0" in content[:8192]:
            continue
        for label, pattern in SECRET_PATTERNS.items():
            if pattern.search(content):
                result.append(f"{name}: contains a {label} signature")
    return result


def tracked_files() -> list[tuple[str, bytes]]:
    names = subprocess.run(
        ["git", "ls-files", "-z"], check=True, stdout=subprocess.PIPE
    ).stdout.split(b"\0")
    files: list[tuple[str, bytes]] = []
    for raw_name in names:
        if not raw_name:
            continue
        name = raw_name.decode("utf-8", "surrogateescape")
        try:
            content = subprocess.run(
                ["git", "show", f":{name}"], check=True, stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
            ).stdout
        except subprocess.CalledProcessError:
            with open(name, "rb") as source:
                content = source.read()
        files.append((name, content))
    return files


def self_test() -> None:
    safe = [(".env.example", b"SETTING=replace-me\n"), ("pkg/testdata/input.json", b"{}")]
    assert findings(safe) == []
    cases = [
        ("workspace-snapshots/snapshot.tar", b"data"),
        ("profiles/chrome-profile/Cookies", b"data"),
        ("dev.kubeconfig", b"data"),
        ("local-data/database.sql", b"data"),
        ("secret.txt", b"-----BEGIN " + b"PRIVATE KEY-----"),
        ("token.txt", b"gh" + b"p_" + b"a" * 36),
        ("cloud.txt", b"AKIA" + b"A" * 16),
    ]
    for case in cases:
        assert findings([case]), f"expected rejection for {case[0]}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    problems = findings(tracked_files())
    if problems:
        print("repository hygiene check failed:", file=sys.stderr)
        for problem in problems:
            print(f"  {problem}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
