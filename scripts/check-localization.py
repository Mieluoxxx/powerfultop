"""Fail when String(localized:) keys and Localizable.strings drift apart.

Keys are the English source strings, so a renamed label silently falls back to English
in zh-Hans and stale keys pile up; both have happened during UI rewrites.
"""
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".")
source = "".join(p.read_text() for p in (root / "PowerTop").rglob("*.swift"))
used = set(re.findall(r'String\(localized: "((?:[^"\\]|\\.)*)"\)', source))

failed = False
for strings in sorted((root / "PowerTop").glob("*.lproj/Localizable.strings")):
    keys = set(re.findall(r'^"((?:[^"\\]|\\.)*)"\s*=', strings.read_text(), re.M))
    for label, diff in (("missing", used - keys), ("unused", keys - used)):
        if diff:
            failed = True
            print(f"{strings.relative_to(root)}: {label} keys: {sorted(diff)}", file=sys.stderr)

if failed:
    sys.exit(1)
print("Localization keys in sync")
