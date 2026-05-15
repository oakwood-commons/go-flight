---
applyTo: "tests/integration/**"
---

# Integration Tests

## Scope
- Integration tests verify cross-package behavior
- They live in `tests/integration/`
- They should run without external dependencies (no network, no databases)

## Naming
- Test functions: `TestIntegration_Feature_Scenario`
- Test files: descriptive names matching the feature being tested

## Guidelines
- Use longer timeouts than unit tests
- Clean up resources in `t.Cleanup()`
- Use subtests for related scenarios
- Document any prerequisites in test file comments
