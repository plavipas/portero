#!/usr/bin/env python3
"""PreToolUse guard: enforce the Portero task flow order per key.

brief -> plan -> developer -> qa -> crossreview -> security
Markers live in .claude/flow-state/<KEY>.<step>, written by flow-mark.
"""
import json
import os
import re
import sys

STATE_DIR = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "flow-state")
FLOW_MARK = os.path.join(os.path.dirname(os.path.abspath(__file__)), "flow-mark")

AGENT_REQUIRES = {
    "architect": "brief",
    "go-developer": "plan",
    "qa-engineer": "developer",
    "security-reviewer": "crossreview",
}
SKILL_REQUIRES = {"cross-review": "qa"}
KEY_RE = re.compile(r"\b(?:DEV(?:OPS)?-\d+|ADHOC-[A-Za-z0-9][A-Za-z0-9-]*)\b", re.IGNORECASE)


def main():
    try:
        event = json.load(sys.stdin)
    except Exception:
        return 0
    tool = event.get("tool_name", "")
    tin = event.get("tool_input", {}) or {}
    if tool in ("Agent", "Task"):
        target = tin.get("subagent_type", "")
        need = AGENT_REQUIRES.get(target)
        text = str(tin.get("description", "")) or str(tin.get("prompt", ""))
        fallback = str(tin.get("prompt", ""))
    elif tool == "Skill":
        target = tin.get("skill", "")
        need = SKILL_REQUIRES.get(target)
        text, fallback = str(tin.get("args", "")), ""
    else:
        return 0
    if not need:
        return 0
    keys = sorted({k.upper() for k in KEY_RE.findall(text)}) or sorted({k.upper() for k in KEY_RE.findall(fallback)})
    if not keys:
        print(f"flow-guard: '{target}' must be launched with the task key (DEV-<n> or DEVOPS-<n>, or ADHOC-<slug> for work "
              f"without an issue yet) in the description/args so the flow order can be checked.", file=sys.stderr)
        return 2
    if len(keys) > 1:
        print(f"flow-guard: '{target}' mentions several keys ({', '.join(keys)}); put the one being worked on "
              f"in the description (Agent) or args (Skill).", file=sys.stderr)
        return 2
    key = keys[0]
    if os.path.exists(os.path.join(STATE_DIR, f"{key}.{need}")):
        return 0
    print(f"flow-guard: {key}: step '{need}' is not completed, so '{target}' cannot run yet. Run the missing step "
          f"first and mark it with '{FLOW_MARK} {key} {need}'. Skipping it is allowed only on the user's explicit "
          f"request: '{FLOW_MARK} {key} {need} --skipped-by-user \"<user's words>\"'.", file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
