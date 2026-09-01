#!/usr/bin/env python3
"""Post ONE advisory COMMENT review to a Forgejo pull request.

The caller (the `forgejo-advisory-review` skill) supplies a summary and a
flat list of findings; this script owns every fiddly part:

  * parse the PR's unified diff into the set of NEW-file line numbers that
    are actually inside a hunk, per file;
  * turn each finding into an inline review comment when its line is in that
    set, and fold every other finding into the review body (prefixed with
    `path:line`) so nothing is silently dropped;
  * POST a single review of type COMMENT, and on Forgejo's all-or-nothing
    422 demote one more inline comment into the body and retry (<= 3x);
  * verify the review actually landed, then print its html_url.

stdout: the review html_url on success.
stderr: one human-readable line describing what was posted, or the failure.
exit:   0 ok | 2 bad input | 3 diff fetch failed | 4 review POST failed
        | 5 posted but could not be verified
"""
from __future__ import annotations

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.request

HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")
MAX_RETRIES = 3


def die(code: int, msg: str):
    sys.stderr.write(f"submit-review: {msg}\n")
    raise SystemExit(code)


def parse_valid_positions(diff_text: str) -> dict[str, set[int]]:
    """Map each file path to the NEW-file line numbers that sit inside a hunk
    (added or context lines). Deleted lines have no NEW-file position."""
    valid: dict[str, set[int]] = {}
    path: str | None = None
    new_lineno = 0
    in_hunk = False
    for line in diff_text.splitlines():
        if line.startswith("diff --git "):
            path, in_hunk = None, False
            continue
        if line.startswith("+++ "):
            target = line[4:].strip()
            if target == "/dev/null":
                path = None
            else:
                path = target[2:] if target[:2] in ("a/", "b/") else target
                valid.setdefault(path, set())
            in_hunk = False
            continue
        if line.startswith("--- "):
            continue
        m = HUNK_RE.match(line)
        if m:
            new_lineno = int(m.group(1))
            in_hunk = path is not None
            continue
        if not in_hunk or path is None:
            continue
        if line.startswith("+"):
            valid[path].add(new_lineno)
            new_lineno += 1
        elif line.startswith("-"):
            pass
        elif line.startswith("\\"):
            pass
        else:  # context line (leading space, or a bare blank context line)
            valid[path].add(new_lineno)
            new_lineno += 1
    return valid


def render_body(summary: str, folded: list[tuple[str, object, str]]) -> str:
    body = summary.rstrip()
    if folded:
        body += "\n\n---\n\n**Findings outside the diff:**\n\n"
        body += "\n".join(
            f"- `{p}:{ln}` — {txt.strip()}" for p, ln, txt in folded
        )
    return body


def api(method: str, url: str, token: str, payload: dict | None = None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"token {token}")
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read()
        return resp.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        try:
            parsed = json.loads(raw) if raw else None
        except ValueError:
            parsed = {"message": raw.decode("utf-8", "replace")}
        return exc.code, parsed


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--pr", required=True, type=int)
    ap.add_argument("--commit", required=True, help="head sha")
    ap.add_argument("--summary-file", required=True)
    ap.add_argument("--comments-file", required=True,
                    help='JSON array of {"path","line","body"}')
    ap.add_argument("--diff-file", help="local unified diff; fetched if omitted")
    ap.add_argument(
        "--api-base",
        default=os.environ.get("FORGEJO_API_BASE", ""),
        help="Forgejo API base, e.g. https://<host>/api/v1; "
             "falls back to $FORGEJO_API_BASE. Required, no default.",
    )
    args = ap.parse_args()

    token = os.environ.get("FORGEJO_TOKEN", "").strip()
    if not token:
        die(2, "FORGEJO_TOKEN is not set")
    if "/" not in args.repo:
        die(2, f"--repo must be owner/name, got {args.repo!r}")
    if not args.api_base.strip():
        die(2, "set --api-base or $FORGEJO_API_BASE (e.g. https://<host>/api/v1)")
    api_base = args.api_base.strip().rstrip("/")

    try:
        with open(args.summary_file, encoding="utf-8") as fh:
            summary = fh.read()
    except OSError as exc:
        die(2, f"cannot read --summary-file: {exc}")
    try:
        with open(args.comments_file, encoding="utf-8") as fh:
            findings = json.load(fh)
    except (OSError, ValueError) as exc:
        die(2, f"cannot read --comments-file: {exc}")
    if not isinstance(findings, list):
        die(2, "--comments-file must contain a JSON array")

    if args.diff_file:
        try:
            with open(args.diff_file, encoding="utf-8") as fh:
                diff_text = fh.read()
        except OSError as exc:
            die(3, f"cannot read --diff-file: {exc}")
    else:
        status, raw = _get_text(
            f"{api_base}/repos/{args.repo}/pulls/{args.pr}.diff", token
        )
        if status != 200 or not raw:
            die(3, f"diff fetch returned HTTP {status}")
        diff_text = raw

    valid = parse_valid_positions(diff_text)

    inline: list[dict] = []
    folded: list[tuple[str, object, str]] = []
    for item in findings:
        if not isinstance(item, dict):
            die(2, f"finding is not an object: {item!r}")
        path = str(item.get("path", "")).strip()
        body = str(item.get("body", "")).strip()
        raw_line = item.get("line")
        try:
            line = int(raw_line)
        except (TypeError, ValueError):
            line = None
        if not path or not body:
            die(2, f"finding needs non-empty path and body: {item!r}")
        if line is not None and line in valid.get(path, set()):
            inline.append({"path": path, "new_position": line, "body": body})
        else:
            folded.append((path, raw_line if line is None else line, body))

    reviews_url = f"{api_base}/repos/{args.repo}/pulls/{args.pr}/reviews"
    attempt = 0
    response = None
    while True:
        payload = {
            "event": "COMMENT",
            "commit_id": args.commit,
            "body": render_body(summary, folded),
            "comments": inline,
        }
        status, response = api("POST", reviews_url, token, payload)
        if status in (200, 201):
            break
        if status == 422 and inline and attempt < MAX_RETRIES:
            demoted = inline.pop()  # no reliable offender index in the 422
            folded.append(
                (demoted["path"], demoted["new_position"], demoted["body"])
            )
            attempt += 1
            sys.stderr.write(
                f"submit-review: 422 from Forgejo; moved comment on "
                f"{demoted['path']}:{demoted['new_position']} into the body "
                f"and retrying ({attempt}/{MAX_RETRIES})\n"
            )
            continue
        msg = ""
        if isinstance(response, dict):
            msg = response.get("message", "")
        die(4, f"review POST failed: HTTP {status} {msg}".rstrip())

    html_url = ""
    review_id = None
    if isinstance(response, dict):
        html_url = response.get("html_url", "")
        review_id = response.get("id")

    status, listing = api("GET", reviews_url, token)
    landed = False
    if status == 200 and isinstance(listing, list):
        for rev in listing:
            if not isinstance(rev, dict):
                continue
            if review_id is not None and rev.get("id") == review_id:
                landed = True
                break
            if rev.get("commit_id") == args.commit and rev.get("html_url") == html_url and html_url:
                landed = True
                break
    if not landed:
        die(5, "review POST returned success but the review could not be "
               f"verified on {reviews_url} (HTTP {status})")

    sys.stderr.write(
        f"submit-review: posted 1 COMMENT review — {len(inline)} inline "
        f"comment(s), {len(folded)} finding(s) in the body\n"
    )
    print(html_url or reviews_url)


def _get_text(url: str, token: str) -> tuple[int, str]:
    req = urllib.request.Request(url, method="GET")
    req.add_header("Authorization", f"token {token}")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, ""


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as exc:  # noqa: BLE001 - last-resort guard
        die(1, f"unexpected error: {exc}")
