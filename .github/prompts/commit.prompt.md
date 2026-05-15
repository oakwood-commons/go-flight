---
description: "Generate a conventional commit message for staged changes"
---

Review the staged changes and generate a conventional commit message.

## Format
```
<type>(<scope>): <description>

[optional body explaining WHY]
```

## Rules
- Type: feat, fix, docs, test, refactor, perf, chore
- Scope: cache, flightgroup, or omit for cross-cutting changes
- Description: lowercase, imperative mood, no period
- Body: explain motivation, not mechanics
