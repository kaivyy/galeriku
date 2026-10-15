"""Validate the Galeriku OpenAPI 3.1 draft: parseable YAML and every $ref resolves.

Usage: python3 backend/scripts/validate_openapi.py GALERIKU_OPENAPI_v1_DRAFT.yaml
Exits 1 with the list of broken references, 0 when the document is clean.
"""

import sys

import yaml


def collect_refs(node, found):
    if isinstance(node, dict):
        for key, value in node.items():
            if key == "$ref" and isinstance(value, str):
                found.add(value)
            else:
                collect_refs(value, found)
    elif isinstance(node, list):
        for item in node:
            collect_refs(item, found)


def resolve(doc, ref):
    current = doc
    for part in ref.lstrip("#/").split("/"):
        if isinstance(current, dict) and part in current:
            current = current[part]
        else:
            return False
    return True


def main():
    if len(sys.argv) != 2:
        print("usage: validate_openapi.py <openapi.yaml>", file=sys.stderr)
        return 2

    with open(sys.argv[1]) as handle:
        doc = yaml.safe_load(handle)

    if doc.get("openapi") != "3.1.0":
        print(f"unexpected openapi version: {doc.get('openapi')}", file=sys.stderr)
        return 1

    refs = set()
    collect_refs(doc, refs)
    broken = sorted(ref for ref in refs if not resolve(doc, ref))

    if broken:
        print(f"{len(broken)} unresolved $ref entries:")
        for ref in broken:
            print(f"  {ref}")
        return 1

    components = doc.get("components", {})
    print(
        "OK: {paths} paths, {schemas} schemas, {refs} refs resolved".format(
            paths=len(doc.get("paths", {})),
            schemas=len(components.get("schemas", {})),
            refs=len(refs),
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
