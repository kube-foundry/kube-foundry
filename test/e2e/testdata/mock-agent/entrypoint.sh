#!/bin/sh
# Mock agent entrypoint for e2e tests.
# Behavior is controlled by the TASK_DESCRIPTION env var.

set -e

TERMINATION_LOG="/tmp/termination-log"

case "$TASK_DESCRIPTION" in
  *MOCK:failure*)
    echo "mock agent: simulating failure" >&2
    echo "Mock agent error: task failed as requested" > "$TERMINATION_LOG"
    exit 1
    ;;
  *MOCK:timeout*)
    echo "mock agent: sleeping forever to simulate timeout" >&2
    sleep 86400
    ;;
  *MOCK:check-skills*)
    echo "mock agent: checking skill injection" >&2
    # Verify SKILL_FILES env var is set and contains expected content
    if [ -z "${SKILL_FILES:-}" ]; then
      echo "SKILL_FILES env var is missing" > "$TERMINATION_LOG"
      exit 1
    fi
    if [ -z "${SKILL_INIT_COMMANDS:-}" ]; then
      echo "SKILL_INIT_COMMANDS env var is missing" > "$TERMINATION_LOG"
      exit 1
    fi
    # Verify skill env vars are passed through
    if [ -z "${SKILL_TEST_VAR:-}" ]; then
      echo "SKILL_TEST_VAR env var is missing" > "$TERMINATION_LOG"
      exit 1
    fi
    echo "https://github.com/example/repo/pull/42" > "$TERMINATION_LOG"
    exit 0
    ;;
  *MOCK:check-skill-missing*)
    echo "mock agent: verifying no skill env vars" >&2
    # This case just verifies the task runs without skills
    if [ -n "${SKILL_FILES:-}" ]; then
      echo "SKILL_FILES should not be set" > "$TERMINATION_LOG"
      exit 1
    fi
    echo "https://github.com/example/repo/pull/42" > "$TERMINATION_LOG"
    exit 0
    ;;
  *MOCK:success*|*)
    echo "mock agent: simulating success" >&2
    echo "https://github.com/example/repo/pull/42" > "$TERMINATION_LOG"
    exit 0
    ;;
esac
