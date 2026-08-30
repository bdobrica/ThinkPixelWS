#!/usr/bin/env python3
"""Dependency-free structural validation for Phase 0 documentation contracts."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"
SCHEMAS = sorted((DOCS / "contracts").glob("*.schema.json"))
REQUIRED = [
    DOCS / "README.md",
    DOCS / "architecture.md",
    DOCS / "security.md",
    DOCS / "database-model.md",
    DOCS / "enterprise-integration.md",
    DOCS / "operations.md",
    DOCS / "portable-snapshot-evaluation.md",
    DOCS / "supported-versions.md",
    DOCS / "contracts/openapi.yaml",
    DOCS / "contracts/providers.md",
    DOCS / "contracts/workspace-manifest.schema.json",
    DOCS / "contracts/portable-snapshot.schema.json",
]


def fail(message: str) -> None:
    print(f"phase0: {message}", file=sys.stderr)
    raise SystemExit(1)


def resolve_pointer(document: object, pointer: str) -> object:
    current = document
    for raw in pointer.removeprefix("#/").split("/"):
        token = raw.replace("~1", "/").replace("~0", "~")
        if not isinstance(current, dict) or token not in current:
            fail(f"unresolved JSON pointer {pointer}")
        current = current[token]
    return current


def walk(value: object):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


def validate_schema(path: Path) -> None:
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot parse {path.relative_to(ROOT)}: {exc}")
    if document.get("$schema") != "https://json-schema.org/draft/2020-12/schema":
        fail(f"{path.name} does not declare JSON Schema 2020-12")
    for node in walk(document):
        ref = node.get("$ref")
        if isinstance(ref, str) and ref.startswith("#/"):
            resolve_pointer(document, ref)
        pattern = node.get("pattern")
        if isinstance(pattern, str):
            try:
                re.compile(pattern)
            except re.error as exc:
                fail(f"invalid regex in {path.name}: {pattern}: {exc}")


def validate_markdown(path: Path) -> None:
    text = path.read_text(encoding="utf-8")
    if text.count("```") % 2:
        fail(f"unbalanced code fence in {path.relative_to(ROOT)}")
    for target in re.findall(r"\[[^]]+\]\(([^)]+)\)", text):
        if target.startswith(("http://", "https://", "#", "mailto:")):
            continue
        clean = target.split("#", 1)[0]
        if clean and not (path.parent / clean).resolve().exists():
            fail(f"broken local link {target!r} in {path.relative_to(ROOT)}")


def validate_openapi_shape(path: Path) -> None:
    text = path.read_text(encoding="utf-8")
    required_tokens = [
        "openapi: 3.1.0", "application/problem+json", "Idempotency-Key",
        "text/event-stream", "oidc:", "commitMaterialization", "forkWorkspace",
        "createPortableSnapshot", "importSource", "archiveWorkspace", "restoreWorkspace",
    ]
    for token in required_tokens:
        if token not in text:
            fail(f"OpenAPI contract is missing {token!r}")
    operation_ids = re.findall(r"^\s+operationId:\s+(\S+)", text, flags=re.MULTILINE)
    if len(operation_ids) != len(set(operation_ids)):
        fail("OpenAPI operationId values are not unique")


def main() -> None:
    missing = [str(path.relative_to(ROOT)) for path in REQUIRED if not path.is_file()]
    if missing:
        fail("missing required artifacts: " + ", ".join(missing))
    for schema in SCHEMAS:
        validate_schema(schema)
    for markdown in [ROOT / "README.md", ROOT / "PLAN.md", ROOT / "TODO.md", *DOCS.rglob("*.md")]:
        validate_markdown(markdown)
    validate_openapi_shape(DOCS / "contracts/openapi.yaml")
    print(f"phase0: validated {len(SCHEMAS)} JSON schemas and {len(list(DOCS.rglob('*.md')))} documentation files")


if __name__ == "__main__":
    main()
