#!/usr/bin/env python3
"""LT-49: CI lint check for the never-log list (observability.md's
"Enforcement beyond 'write it carefully'" section).

Not foolproof by design (a determined developer can rename around it),
per observability.md's own framing -- this catches the accidental
case: a whole request/response struct or an *http.Request passed
straight into a log call, a %+v/%#v format verb on a Go log call (the
classic "dump the whole struct" anti-pattern), or a log-call argument
whose identifier name matches a never-log-list field name.

Scans internal/ and cmd/ only -- test files are exempt, since a
never-log-list enforcement TEST (LT-49's own 03-side criterion) has to
seed a real sensitive-looking value on purpose to prove the real code
path won't leak it; flagging that here would be exactly the false
positive this script exists to avoid.
"""
import re
import sys
from pathlib import Path

LOG_CALL = re.compile(
    r"\b(?:log\.Printf|log\.Println|log\.Print|slog\.(?:Info|Warn|Error|Debug|Log))\s*\("
)

# %+v/%#v are the whole-struct-dump format verbs this rule exists to catch --
# %v alone is too broad (used constantly for plain scalars, error values,
# etc.) and would make this check useless noise.
STRUCT_DUMP_VERB = re.compile(r"%[+#]v")

# Case-insensitive identifier match on a never-log-list field name used as
# a bare Go identifier (not a string key) inside a log call's argument
# list -- e.g. `slog.Info("token issued", token)` should flag, but
# `slog.Info("grant", "client_id", clientID)` (a labeled, non-sensitive
# value) should not.
NEVER_LOG_FIELD_NAMES = [
    "password",
    "token",
    "secret",
    "dsn",
    "client_secret",
]
BARE_IDENTIFIER = re.compile(
    r"\b(" + "|".join(re.escape(n) for n in NEVER_LOG_FIELD_NAMES) + r")\w*\b",
    re.IGNORECASE,
)

WHOLE_REQUEST_ARG = re.compile(r"\br\.Body\b|\breq\b\s*[,)]|\*http\.Request\b")

SCAN_DIRS = ["internal", "cmd"]


def is_test_file(path: Path) -> bool:
    return path.name.endswith("_test.go")


def find_log_call_spans(text: str):
    """Yield (start, end) character spans covering each log call's full
    argument list, by counting parens from the call's opening '(' --
    handles calls that span multiple lines or nest other calls."""
    for m in LOG_CALL.finditer(text):
        depth = 1
        i = m.end()
        start = m.start()
        while i < len(text) and depth > 0:
            if text[i] == "(":
                depth += 1
            elif text[i] == ")":
                depth -= 1
            i += 1
        yield start, i, text[start:i]


STRING_LITERAL = re.compile(r'"(?:[^"\\]|\\.)*"|`[^`]*`')


def _blank_string_literals(span: str) -> str:
    """Replace the contents of every Go string literal with '#' padding
    (same length, so character offsets/line numbers stay correct) --
    a never-log-list field name appearing only inside a log MESSAGE
    string (e.g. "...token will return server_error...") is not a bare
    identifier argument and must not be flagged; only a real Go
    identifier/expression argument should be."""
    return STRING_LITERAL.sub(lambda m: "#" * len(m.group(0)), span)


def check_file(path: Path) -> list[str]:
    findings = []
    text = path.read_text()
    lineno_at = _line_index(text)
    for start, _end, span in find_log_call_spans(text):
        line = lineno_at(start)
        code_only = _blank_string_literals(span)
        if STRUCT_DUMP_VERB.search(span):
            findings.append(
                f"{path}:{line}: log call uses a whole-struct dump verb (%+v/%#v) -- "
                f"log explicit fields instead"
            )
        if WHOLE_REQUEST_ARG.search(code_only):
            findings.append(
                f"{path}:{line}: log call appears to pass a whole request/response "
                f"value -- log explicit fields instead"
            )
        for bare_match in BARE_IDENTIFIER.finditer(code_only):
            findings.append(
                f"{path}:{line}: log call argument matches never-log-list field "
                f"name '{bare_match.group(0)}' -- confirm this isn't the raw "
                f"sensitive value (rename or redact if it is)"
            )
    return findings


def _line_index(text: str):
    offsets = [0]
    for i, c in enumerate(text):
        if c == "\n":
            offsets.append(i + 1)
    def at(pos: int) -> int:
        lo, hi = 0, len(offsets) - 1
        while lo < hi:
            mid = (lo + hi + 1) // 2
            if offsets[mid] <= pos:
                lo = mid
            else:
                hi = mid - 1
        return lo + 1
    return at


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    findings: list[str] = []
    for d in SCAN_DIRS:
        for path in (root / d).rglob("*.go"):
            if is_test_file(path):
                continue
            findings.extend(check_file(path))
    if findings:
        for f in findings:
            print(f"::error::{f}")
        print(f"\n{len(findings)} never-log-list violation(s) found.", file=sys.stderr)
        return 1
    print("No never-log-list violations found.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
