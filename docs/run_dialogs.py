#!/usr/bin/env python3
"""
run_dialogs.py – Batch-runner for Helpline LangGraph demo

Reads a YAML file containing one or more dialogs and, for each user-assistant
turn, invokes an existing helper script (e.g. `helpline_langgraph.py`) that
accepts a single user prompt on its command line and prints the assistant reply
to stdout.

Usage
-----
    python run_dialogs.py helpline_data.yaml helpline_langgraph.py

* helpline_data.yaml  – input file with dialogs (same format you uploaded)
* helpline_langgraph.py  – the working assistant script; default if omitted
"""
from __future__ import annotations

import subprocess
import sys
from pathlib import Path
from typing import List, Dict

import yaml


# ── helpers ────────────────────────────────────────────────────────────────
def run_turn(helper: Path, user_prompt: str) -> str:
    """Invoke the helper script with the prompt and capture stdout."""
    result = subprocess.run(
        [sys.executable, str(helper), user_prompt],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"Helper script exited with {result.returncode}:\n{result.stderr}"
        )
    return result.stdout.strip()


def run_dialog(helper: Path, dialog: List[Dict[str, str]], index: int) -> None:
    print(f"Running dialog #{index}\n{'-'*40}")
    for turn in dialog:
        user_in = turn.get("input", "")
        expected = turn.get("output", "")
        assistant = run_turn(helper, user_in)
        print(f"User:      {user_in}")
        print(f"Expected:  {expected}")
        print(f"Assistant: {assistant}\n")
    print("=" * 40 + "\n")


# ── main ───────────────────────────────────────────────────────────────────
if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python run_dialogs.py <yaml-file> [helper-script]")
        sys.exit(1)

    yaml_path = Path(sys.argv[1])
    helper_path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("helpline_langgraph.py")

    if not yaml_path.exists():
        sys.exit(f"YAML file not found: {yaml_path}")
    if not helper_path.exists():
        sys.exit(f"Helper script not found: {helper_path}")

    with yaml_path.open("r", encoding="utf-8") as f:
        data = yaml.safe_load(f)

    scripts = data.get("scripts", [])
    if not scripts:
        sys.exit("No scripts found in YAML under key 'scripts'.")

    for idx, script in enumerate(scripts, 1):
        dialog = script.get("dialog", [])
        if not dialog:
            print(f"(Skipping empty dialog #{idx})")
            continue
        run_dialog(helper_path, dialog, idx)