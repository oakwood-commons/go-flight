#!/usr/bin/env bash
# Git safety hook — prevents accidental commits/pushes by AI agents
# This hook runs before tool use to block dangerous git operations

set -euo pipefail

# Read the tool invocation from environment (if available).
# These variables are only set by AI agent runtimes (e.g., Copilot).
# When absent, the hook is intentionally a no-op — it should not
# block human developers from using git normally.
TOOL_NAME="${COPILOT_TOOL_NAME:-}"
TOOL_INPUT="${COPILOT_TOOL_INPUT:-}"

# Block dangerous git operations
if echo "$TOOL_INPUT" | grep -qE '(git\s+(commit|push|reset\s+--hard|rebase|force-push|merge))|--force'; then
  echo "⚠️  BLOCKED: Git write operation detected."
  echo "   Tool: $TOOL_NAME"
  echo "   Please confirm with the user before running git commit/push operations."
  exit 1
fi

exit 0
