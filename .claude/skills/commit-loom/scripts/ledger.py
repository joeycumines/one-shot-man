#!/usr/bin/env python3
"""ledger.py — Manage the commit-loom ledger JSON file.

Usage:
    python scripts/ledger.py init --patch <path> --backup <branch> --head <sha> [options]
    python scripts/ledger.py remaining <ledger-path>
    python scripts/ledger.py advance <ledger-path> --state <new-state> --cycle <N>
    python scripts/ledger.py record-commit <ledger-path> --cycle <N> --sha <sha> --theme <theme>
    python scripts/ledger.py disposition <ledger-path> --hunk-id <ID> --action <action> [--note <note>] [--commit-id <ID>]
    python scripts/ledger.py register-hunk <ledger-path> --hunk-id <ID> --file <path> --start-line <N> --end-line <N> [--summary <text>]
    python scripts/ledger.py set-discovery <ledger-path> --key <key> --value <value>
    python scripts/ledger.py set-baseline <ledger-path> --stash-ref <ref> --check-results <json> --pre-existing-failures <json>
    python scripts/ledger.py summary <ledger-path>
"""

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def load_ledger(path: str) -> dict:
    p = Path(path)
    if not p.exists():
        print(f"Error: ledger not found at {path}", file=sys.stderr)
        sys.exit(1)
    with open(p) as f:
        return json.load(f)


def save_ledger(path: str, data: dict) -> None:
    data["meta"]["updated_at"] = datetime.now(timezone.utc).isoformat()
    # Update baseline timestamp if a baseline has been established
    baseline = data.get("baseline")
    if isinstance(baseline, dict):
        baseline["updated_at"] = datetime.now(timezone.utc).isoformat()
    # Update resume guide counts
    rg = data.get("_resume_guide", {})
    consumed = sum(1 for h in data.get("hunk_registry", []) if h.get("status") == "consumed")
    total = len(data.get("hunk_registry", []))
    rg["consumed_hunks"] = consumed
    rg["remaining_hunks"] = total - consumed
    rg["current_cycle"] = data.get("plan", {}).get("_next_cycle", 1)
    completed = sum(1 for c in data.get("plan", {}).get("planned_commits", []) if c.get("status") == "DONE")
    rg["total_cycles_completed"] = completed
    rg["total_hunks"] = total

    with open(path, "w") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.write("\n")


def cmd_init(args):
    """Initialize a new ledger file."""
    now = datetime.now(timezone.utc)
    timestamp = now.strftime("%Y%m%d-%H%M%S")
    ledger_path = f"./scratch/commit-loom-{timestamp}.json"

    ledger = {
        "_resume_guide": {
            "skill": "commit-loom",
            "version": 1,
            "what_is_this": "Operational state for a commit-loom run. Read this file to resume.",
            "current_state": "CAPTURE",
            "current_cycle": 0,
            "total_cycles_completed": 0,
            "total_hunks": 0,
            "consumed_hunks": 0,
            "remaining_hunks": 0,
            "branches": {
                "backup": args.backup,
                "working": args.branch or "main",
            },
            "snapshot_file": args.patch,
            "instructions": [
                "Read this file fully.",
                "Check current_state for the state machine position.",
                "Read the plan section for remaining work.",
                "Resume from the cycle indicated by current_cycle.",
            ],
            "reread_reminders": {
                "every_cycle_start": "Re-read _resume_guide and plan sections",
                "before_verify": "Re-read discovery.tooling_preferences and discovery.check_suite",
                "before_external_skill": "Re-read that skill's full instructions",
            },
            "key_reminders": [
                "Tooling preferences (discovery.tooling_preferences): READ THIS BEFORE RUNNING CHECKS. Context decay causes wrong tooling choices."
            ],
        },
        "meta": {
            "started_at": now.isoformat(),
            "updated_at": now.isoformat(),
            "repository_root": str(Path.cwd()),
            "original_head": args.head,
            "original_head_message": args.head_message or "",
            "snapshot_patch": args.patch,
            "snapshot_stat": {
                "files_changed": int(args.files or 0),
                "insertions": int(args.insertions or 0),
                "deletions": int(args.deletions or 0),
            },
        },
        "discovery": {
            "check_suite": {},
            "tooling_preferences": {
                "package_manager": None,
                "never_use": [],
                "always_use": [],
                "build_before_test": {"required": False},
                "mcps": [],
                "codebase_tools": [],
                "custom_scripts": [],
                "ci_uses": [],
            },
            "ways_of_working": {},
            "review_pattern": None,
            "available_skills": [],
            "available_agents": [],
            "validation_notes": "",
        },
        "baseline": None,
        "plan": {
            "goal_state": "All hunks consumed via a sequence of self-sufficient, validated commits.",
            "initial_hunk_count": int(args.hunks or 0),
            "initial_file_count": int(args.files or 0),
            "planned_commits": [],
            "_next_cycle": 1,
            "replan_count": 0,
            "replan_log": [],
        },
        "hunk_registry": [],
        "cycle_log": [],
        "final_summary": None,
    }

    Path("./scratch").mkdir(exist_ok=True)
    save_ledger(ledger_path, ledger)
    print(ledger_path)


def cmd_remaining(args):
    """List unconsumed hunks."""
    ledger = load_ledger(args.ledger)
    unconsumed = [h for h in ledger.get("hunk_registry", []) if h.get("status") == "unconsumed"]
    print(json.dumps(unconsumed, indent=2))


def cmd_advance(args):
    """Advance the state machine."""
    ledger = load_ledger(args.ledger)
    ledger["_resume_guide"]["current_state"] = args.state
    ledger["plan"]["_next_cycle"] = int(args.cycle)
    save_ledger(args.ledger, ledger)
    print(f"Advanced to {args.state}, cycle {args.cycle}")


def cmd_record_commit(args):
    """Record a completed commit."""
    ledger = load_ledger(args.ledger)
    cycle = int(args.cycle)

    # Find or create the commit entry
    commits = ledger["plan"]["planned_commits"]
    commit_entry = None
    for c in commits:
        if c.get("id") == cycle:
            commit_entry = c
            break

    if commit_entry is None:
        commit_entry = {"id": cycle, "theme": args.theme, "status": "DONE"}
        commits.append(commit_entry)

    commit_entry["status"] = "DONE"
    commit_entry["commit_sha"] = args.sha
    commit_entry["commit_message"] = args.message or ""

    # Add cycle log entry (skip if one already exists for this cycle)
    now = datetime.now(timezone.utc).isoformat()
    existing = any(e.get("cycle") == cycle for e in ledger.get("cycle_log", []))
    if not existing:
        ledger["cycle_log"].append({
            "cycle": cycle,
            "started_at": now,
            "ended_at": now,
            "state_transitions": [],
            "commit_id": cycle,
            "commit_sha": args.sha,
            "validation_results": {},
            "issues_found": [],
            "fixes_applied": [],
            "duration_seconds": 0,
            "notes": "",
        })

    # Advance next cycle
    ledger["plan"]["_next_cycle"] = cycle + 1
    ledger["_resume_guide"]["current_state"] = "UPDATE"

    save_ledger(args.ledger, ledger)
    print(f"Recorded commit {args.sha} for cycle {cycle}")


def cmd_disposition(args):
    """Set disposition for a hunk."""
    ledger = load_ledger(args.ledger)
    hunk_id = int(args.hunk_id)

    for h in ledger.get("hunk_registry", []):
        if h.get("id") == hunk_id:
            h["disposition"] = {
                "action": args.action,
                "commit_id": int(args.commit_id) if args.commit_id else None,
                "note": args.note,
            }
            h["status"] = "consumed"
            save_ledger(args.ledger, ledger)
            print(f"Hunk {hunk_id}: {args.action}")
            return

    print(f"Error: hunk {hunk_id} not found", file=sys.stderr)
    sys.exit(1)


def cmd_summary(args):
    """Render the final summary."""
    ledger = load_ledger(args.ledger)

    commits = [c for c in ledger["plan"]["planned_commits"] if c.get("status") == "DONE"]
    hunks = ledger.get("hunk_registry", [])

    disposition_counts = {}
    for h in hunks:
        d = h.get("disposition")
        if d:
            action = d.get("action", "unknown")
            disposition_counts[action] = disposition_counts.get(action, 0) + 1
        else:
            disposition_counts["no_disposition"] = disposition_counts.get("no_disposition", 0) + 1

    summary = {
        "completed_at": datetime.now(timezone.utc).isoformat(),
        "total_commits": len(commits),
        "commit_shas": [c.get("commit_sha", "") for c in commits],
        "commit_summaries": [c.get("theme", "") for c in commits],
        "hunk_accounting": {
            "total": len(hunks),
            **disposition_counts,
        },
    }

    print(json.dumps(summary, indent=2))


def cmd_register_hunk(args):
    """Register a hunk in the hunk_registry."""
    ledger = load_ledger(args.ledger)
    hunk = {
        "id": int(args.hunk_id),
        "file": args.file,
        "start_line": int(args.start_line),
        "end_line": int(args.end_line),
        "summary": args.summary or "",
        "original_content_hash": args.hash or "",
        "disposition": None,
        "status": "unconsumed",
    }
    ledger["hunk_registry"].append(hunk)
    save_ledger(args.ledger, ledger)
    print(f"Registered hunk {args.hunk_id}: {args.file}:{args.start_line}-{args.end_line}")


def cmd_set_discovery(args):
    """Set a discovery field."""
    ledger = load_ledger(args.ledger)
    key = args.key
    value = args.value

    if key not in ledger.get("discovery", {}):
        print(f"Warning: discovery key '{key}' not found in ledger, adding anyway", file=sys.stderr)

    # Handle JSON-like values (lists, bools, numbers)
    if value.lower() == "true":
        value = True
    elif value.lower() == "false":
        value = False
    elif value.startswith("["):
        import json as _json
        try:
            value = _json.loads(value)
        except _json.JSONDecodeError:
            pass
    else:
        try:
            value = int(value)
        except ValueError:
            pass

    ledger.setdefault("discovery", {})[key] = value
    save_ledger(args.ledger, ledger)
    print(f"Set discovery.{key} = {value!r}")


def cmd_set_baseline(args):
    """Establish the check baseline against the clean tree."""
    ledger = load_ledger(args.ledger)

    check_results = json.loads(args.check_results)
    pre_existing_failures = json.loads(args.pre_existing_failures)

    ledger["baseline"] = {
        "established_at": datetime.now(timezone.utc).isoformat(),
        "stash_ref": args.stash_ref,
        "check_results": check_results,
        "pre_existing_failures": pre_existing_failures,
    }

    save_ledger(args.ledger, ledger)
    print(f"Baseline established (stash: {args.stash_ref})")


def main():
    parser = argparse.ArgumentParser(description="Manage commit-loom ledger")
    sub = parser.add_subparsers(dest="command")

    # init
    p_init = sub.add_parser("init")
    p_init.add_argument("--patch", required=True)
    p_init.add_argument("--backup", required=True)
    p_init.add_argument("--head", required=True)
    p_init.add_argument("--head-message", default=None)
    p_init.add_argument("--branch", default=None)
    p_init.add_argument("--hunks", default="0")
    p_init.add_argument("--files", default="0")
    p_init.add_argument("--insertions", default="0")
    p_init.add_argument("--deletions", default="0")

    # remaining
    p_rem = sub.add_parser("remaining")
    p_rem.add_argument("ledger")

    # advance
    p_adv = sub.add_parser("advance")
    p_adv.add_argument("ledger")
    p_adv.add_argument("--state", required=True)
    p_adv.add_argument("--cycle", required=True)

    # record-commit
    p_rc = sub.add_parser("record-commit")
    p_rc.add_argument("ledger")
    p_rc.add_argument("--cycle", required=True)
    p_rc.add_argument("--sha", required=True)
    p_rc.add_argument("--theme", default="")
    p_rc.add_argument("--message", default=None)

    # disposition
    p_disp = sub.add_parser("disposition")
    p_disp.add_argument("ledger")
    p_disp.add_argument("--hunk-id", required=True)
    p_disp.add_argument("--action", required=True)
    p_disp.add_argument("--note", default=None)
    p_disp.add_argument("--commit-id", default=None)

    # summary
    p_sum = sub.add_parser("summary")
    p_sum.add_argument("ledger")

    # register-hunk
    p_reg = sub.add_parser("register-hunk")
    p_reg.add_argument("ledger")
    p_reg.add_argument("--hunk-id", required=True)
    p_reg.add_argument("--file", required=True)
    p_reg.add_argument("--start-line", required=True)
    p_reg.add_argument("--end-line", required=True)
    p_reg.add_argument("--summary", default=None)
    p_reg.add_argument("--hash", default=None)

    # set-discovery
    p_disc = sub.add_parser("set-discovery")
    p_disc.add_argument("ledger")
    p_disc.add_argument("--key", required=True)
    p_disc.add_argument("--value", required=True)

    # set-baseline
    p_bl = sub.add_parser("set-baseline")
    p_bl.add_argument("ledger")
    p_bl.add_argument("--stash-ref", required=True)
    p_bl.add_argument("--check-results", required=True)
    p_bl.add_argument("--pre-existing-failures", required=True)

    args = parser.parse_args()
    if args.command == "init":
        cmd_init(args)
    elif args.command == "remaining":
        cmd_remaining(args)
    elif args.command == "advance":
        cmd_advance(args)
    elif args.command == "record-commit":
        cmd_record_commit(args)
    elif args.command == "disposition":
        cmd_disposition(args)
    elif args.command == "summary":
        cmd_summary(args)
    elif args.command == "register-hunk":
        cmd_register_hunk(args)
    elif args.command == "set-discovery":
        cmd_set_discovery(args)
    elif args.command == "set-baseline":
        cmd_set_baseline(args)
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
