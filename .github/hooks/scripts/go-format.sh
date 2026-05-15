#!/usr/bin/env bash
# Auto-format Go files after edits
# Runs goimports + gofumpt on modified .go files

set -euo pipefail

# Only process if .go files were edited
MODIFIED_FILE="${COPILOT_TOOL_OUTPUT_FILE:-}"

if [[ -z "$MODIFIED_FILE" ]] || [[ "$MODIFIED_FILE" != *.go ]]; then
  exit 0
fi

if [[ ! -f "$MODIFIED_FILE" ]]; then
  exit 0
fi

# Try to format with goimports and gofumpt if available
if command -v goimports >/dev/null 2>&1; then
  goimports -w "$MODIFIED_FILE" 2>/dev/null || true
fi

if command -v gofumpt >/dev/null 2>&1; then
  gofumpt -w -extra "$MODIFIED_FILE" 2>/dev/null || true
fi

exit 0
