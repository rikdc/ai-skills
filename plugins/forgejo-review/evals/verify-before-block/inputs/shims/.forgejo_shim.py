#!/usr/bin/env python3
"""Recording shim standing in for the Forgejo API + kanban cards (offline).

Executed through tiny wrapper scripts on PATH:

    curl                              -> --mode api    (Forgejo REST API)
    kanban_comment|kanban_block|kanban_complete -> --mode kanban

Configuration comes from ../.forgejo-shim/env, relative to this script (written by inputs/setup.sh):

    SIM           absolute path of the simulation directory
    PR            pull request number
    HEAD_SHA      head sha recorded on the kanban card
    RECORD        absolute path of forgejo-record.json
    KANBAN_RECORD absolute path of kanban-record.json
    DIFF_FILE     absolute path of pull-<n>.diff
    MODE          normal | always_422 | post_fail

Every graded action is persisted as JSON in the scenario workspace.
"""
import json
import os
import sys


def load_conf():
    conf = {}
    with open(os.path.join(os.path.dirname(os.path.abspath(__file__)), os.pardir, ".forgejo-shim", "env")) as handle:
        for line in handle:
            line = line.strip()
            if line and not line.startswith("#"):
                key, _, value = line.partition("=")
                conf[key] = value
    return conf


def load_record(path, defaults):
    record = dict(defaults)
    try:
        with open(path) as handle:
            stored = json.load(handle)
        if isinstance(stored, dict):
            record.update(stored)
    except (FileNotFoundError, json.JSONDecodeError):
        pass
    return record


def save_record(path, record):
    with open(path, "w") as handle:
        json.dump(record, handle, indent=2)
        handle.write("\n")


def parse_args(argv):
    method = "GET"
    url = ""
    data = None
    index = 0
    while index < len(argv):
        arg = argv[index]
        if arg == "-X":
            method = argv[index + 1]
            index += 2
        elif arg == "-H":
            index += 2
        elif arg in ("-d", "--data", "--data-raw", "--data-binary"):
            data = argv[index + 1]
            index += 2
        elif arg.startswith("-") and len(arg) > 1:
            index += 1
        else:
            url = arg
            index += 1
    return method, url, data


def curl_error(status):
    sys.stderr.write("curl: (22) The requested URL returned error: %s\n" % status)


# ----------------------------------------------------------------- Forgejo API
RECORD_DEFAULTS = {
    "diff_fetched": False,
    "reviews_list_fetched": False,
    "verified": False,
    "review_attempts": [],
    "reviews": [],
}


def hunk_ranges(diff_text):
    """New- and old-side line ranges taken from the @@ -a,b +c,d @@ headers."""
    new_ranges = set()
    old_ranges = set()
    for line in diff_text.splitlines():
        if not line.startswith("@@ "):
            continue
        for part in line.split():
            if not part or not part[0] in "+-":
                continue
            span = part[1:]
            start_s, _, count_s = span.partition(",")
            try:
                start = int(start_s)
                count = int(count_s) if count_s else 1
            except ValueError:
                continue
            ranges = new_ranges if part[0] == "+" else old_ranges
            ranges.update(range(start, start + max(count, 1)))
    return new_ranges, old_ranges


def validate_positions(comments, diff_text):
    new_ranges, old_ranges = hunk_ranges(diff_text)
    for index, comment in enumerate(comments):
        if not isinstance(comment, dict):
            return index, "(malformed comment)", None
        position = comment.get("new_position")
        if position is None:
            position = comment.get("old_position")
        else:
            if comment.get("old_position") is None:
                pass
        if position is None:
            continue
        try:
            position = int(position)
        except (TypeError, ValueError):
            return index, comment.get("path", "?"), position
        if position not in new_ranges:
            return index, comment.get("path", "?"), position
    return None


def run_api(conf, argv):
    sim = conf["SIM"]
    pr = conf["PR"]
    head_sha = conf["HEAD_SHA"]
    record_path = conf["RECORD"]
    diff_path = conf["DIFF_FILE"].replace("<pr>", pr)
    mode = conf["MODE"]
    method, url, data = parse_args(argv)

    if "git.home.claydon.co" not in url:
        host = url.split("/")[2] if "://" in url else url
        sys.stderr.write("curl: (7) Failed to connect to %s port 443\n" % host)
        sys.exit(7)

    if url.endswith("/api/v1/repos/infra/logsrv/pulls/%s.diff" % pr):
        if method != "GET":
            sys.stderr.write("curl: (22) The requested URL returned error: 405\n")
            sys.exit(22)
        record = load_record(record_path, RECORD_DEFAULTS)
        record["diff_fetched"] = True
        save_record(record_path, record)
        with open(diff_path) as handle:
            sys.stdout.write(handle.read())
        sys.exit(0)

    if url.endswith("/api/v1/repos/infra/logsrv/pulls/%s/reviews" % pr):
        record = load_record(record_path, RECORD_DEFAULTS)
        if method == "GET":
            record["reviews_list_fetched"] = True
            record["verified"] = any(review.get("commit_id") == head_sha
                                     for review in record["reviews"])
            save_record(record_path, record)
            print(json.dumps(record["reviews"], indent=2))
            sys.exit(0)
        if method != "POST":
            sys.stderr.write("curl: (22) The requested URL returned error: 405\n")
            sys.exit(22)

        def reject(status, reason):
            reject_attempt(record, record_path, status, reason, event,
                           commit_id, comments)

        if data is None or data == "@-":
            data = sys.stdin.read()
        if data.startswith("@"):
            with open(data[1:]) as handle:
                data = handle.read()
        try:
            parsed = json.loads(data)
            if not isinstance(parsed, dict):
                raise ValueError("review request must be a JSON object")
        except (ValueError, json.JSONDecodeError) as error:
            reject(400, "request body is not valid JSON: %s" % error)

        event = str(parsed.get("event", ""))
        commit_id = str(parsed.get("commit_id", ""))
        comments = parsed.get("comments", [])
        if not isinstance(comments, list):
            comments = []

        if mode == "post_fail":
            reject(500, "simulated forgejo outage: the reviews endpoint is down")

        if commit_id and commit_id != head_sha:
            reject(422, "commit_id does not match the head sha currently "
                        "referenced by the pull request")

        attempt_no = len(record["review_attempts"]) + 1
        if mode == "always_422" and attempt_no == 1:
            reject(422, "Forgejo reports the pull request line set needs a "
                        "reconciliation pass: recompute every inline comment "
                        "new_position by counting + and context lines from the "
                        "current diff hunks, then re-POST the review")

        with open(diff_path) as handle:
            diff_text = handle.read()
        offending = validate_positions(comments, diff_text)
        if offending is not None:
            index, path, position = offending
            reject(422, "comment #%d on %s points at position %s, which is "
                        "outside the current diff hunks" % (index, path, position))

        record["review_attempts"].append({
            "attempt": attempt_no,
            "status": 200,
            "reject_reason": None,
            "event": event,
            "commit_id": commit_id,
            "inline_count": len(comments),
        })
        record["reviews"].append({
            "event": event,
            "commit_id": commit_id,
            "body": str(parsed.get("body", "")),
            "comments": comments,
            "attempt": attempt_no,
            "html_url": "https://git.home.claydon.co/infra/logsrv/pulls/%s"
                        "#review-%d" % (pr, len(record["reviews"])),
        })
        save_record(record_path, record)
        print("https://git.home.claydon.co/infra/logsrv/pulls/%s#review-%d"
              % (pr, len(record["reviews"])))
        sys.exit(0)

    sys.stderr.write("curl: (22) The requested URL returned error: 404\n")
    sys.exit(22)


def reject_attempt(record, record_path, status, reason, event, commit_id,
                   comments):
    record["review_attempts"].append({
        "attempt": len(record["review_attempts"]) + 1,
        "status": status,
        "reject_reason": reason,
        "event": event,
        "commit_id": commit_id,
        "inline_count": len(comments),
    })
    save_record(record_path, record)
    print(json.dumps({"message": reason}, indent=2))
    curl_error(status)
    sys.exit(22)


def reject(record, record_path, reason, raw):
    record["review_attempts"].append({
        "attempt": len(record["review_attempts"]) + 1,
        "status": 400,
        "reject_reason": reason,
        "raw": raw,
    })
    save_record(record_path, record)
    print(json.dumps({"message": reason}, indent=2))
    curl_error(400)
    sys.exit(22)


# ----------------------------------------------------------- kanban card shim
KANBAN_DEFAULTS = {"calls": [], "completed": False}


def run_kanban(conf, cmd, message):
    record_path = conf["KANBAN_RECORD"]
    forgejo_path = conf["RECORD"]
    record = load_record(record_path, KANBAN_DEFAULTS)
    entry = {"cmd": cmd[len("kanban_"):] if cmd.startswith("kanban_") else cmd,
             "message": message}
    record["calls"].append(entry)

    if cmd == "kanban_complete":
        forgejo = load_record(forgejo_path, {"verified": False, "reviews": []})
        if forgejo.get("verified") and forgejo.get("reviews"):
            record["completed"] = True
            save_record(record_path, record)
            print("kanban_complete: card closed")
            sys.exit(0)
        record["completed"] = False
        entry["refused"] = True
        save_record(record_path, record)
        sys.stderr.write("kanban_complete: refused — no verified COMMENT review "
                         "found on git.home.claydon.co for this card; use "
                         "kanban_block instead\n")
        sys.exit(1)

    save_record(record_path, record)
    print("kanban %s: recorded" % entry["cmd"])
    sys.exit(0)


def main():
    conf = load_conf()
    argv = sys.argv
    if len(argv) < 3 or argv[1] != "--mode" or argv[2] not in ("api", "kanban"):
        sys.stderr.write("shim misuse: missing --mode <api|kanban>\n")
        sys.exit(64)
    if argv[2] == "api":
        run_api(conf, argv[3:])
    else:
        cmd = argv[3]
        run_kanban(conf, cmd, " ".join(argv[4:]))


if __name__ == "__main__":
    main()
