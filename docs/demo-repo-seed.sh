#!/usr/bin/env bash
# Seed a dedicated demo repo for the lazyboards README GIF.
#
# Usage:
#   1. Create the repo first:  gh repo create <you>/lazyboards-demo --public --clone
#   2. cd lazyboards-demo
#   3. Run this script:  bash /path/to/lazyboards/docs/demo-repo-seed.sh
#
# Re-run to top up state after issues drift. Idempotent for labels;
# issues are always appended, so clear the repo (or delete issues) first
# if you want a clean slate.

set -euo pipefail

if ! gh repo view >/dev/null 2>&1; then
  echo "error: run this inside a cloned gh repo" >&2
  exit 1
fi

echo "==> Ensuring labels exist"
create_label() {
  local name="$1" color="$2" desc="${3:-}"
  gh label create "$name" --color "$color" --description "$desc" 2>/dev/null \
    || gh label edit   "$name" --color "$color" --description "$desc" >/dev/null
}

# Column labels — match lazyboards' default column names
create_label "New"          "c5def5" "Incoming, not yet triaged"
create_label "Refined"      "bfd4f2" "Ready to be worked on"
create_label "Implementing" "fbca04" "In progress"
create_label "Implemented"  "0e8a16" "Done"

# Extra labels for flavour
create_label "bug"          "d73a4a" "Something isn't working"
create_label "enhancement"  "a2eeef" "New feature or request"
create_label "docs"         "0075ca" "Documentation change"

echo "==> Creating issues"

new_issue() {
  local title="$1" label="$2" body="$3"
  gh issue create --title "$title" --label "$label" --body "$body" >/dev/null
  echo "  + $title  [$label]"
}

# ---- New (3) ----
new_issue "Dark mode toggle in settings" \
  "New,enhancement" \
  "Users want to switch between light and dark themes at runtime without restarting the app."

new_issue "Export notes as Markdown" \
  "New,enhancement" \
  "Add a bulk export action that writes every note to a folder of \`.md\` files with YAML frontmatter."

new_issue "Crash when opening empty workspace" \
  "New,bug" \
  "Steps to reproduce:
1. Launch the app with no workspace argument
2. Close the welcome modal
3. Nil pointer panic in \`workspace.Current()\`

Stack trace attached. Regression since 0.4.2."

# ---- Refined (3) ----
new_issue "Support nested tags (tag/subtag)" \
  "Refined,enhancement" \
  "## Summary

Allow tags like \`project/alpha\` to group under a parent \`project\` node in the sidebar.

## Acceptance criteria

- Parser splits on \`/\` into a tree
- Sidebar renders expandable nodes
- Collapsing state persists across restarts
- Backwards compatible with flat tags"

new_issue "Keyboard shortcut for quick switcher" \
  "Refined,enhancement" \
  "Bind \`Ctrl+P\` to open a fuzzy-find palette across notes and tags. Reuse the existing command palette component."

new_issue "Replace deprecated encoding/json with v2" \
  "Refined" \
  "Swap \`encoding/json\` for \`encoding/json/v2\` where available. Benchmarks show a 30-40% decode speedup on our largest fixtures."

# ---- Implementing (2) ----
new_issue "Sync engine: conflict resolution UI" \
  "Implementing,enhancement" \
  "## Problem

When two devices edit the same note offline, the current sync engine picks last-write-wins silently. Users have asked for a three-way merge UI.

## Plan

1. Detect conflict on pull
2. Show side-by-side diff
3. Let the user keep either side or edit a merged version
4. Emit a sync event for observability

## Progress

- [x] Conflict detection
- [x] Diff renderer
- [ ] Merge editor
- [ ] End-to-end tests"

new_issue "Fix flaky test in auth middleware" \
  "Implementing,bug" \
  "\`TestAuth_RejectsExpiredToken\` fails ~1 in 20 runs on CI. Suspected race in the token cache eviction goroutine."

# ---- Implemented (4) ----
new_issue "Add JSON logging mode" \
  "Implemented,enhancement" \
  "Structured logs for production environments. Enabled via \`LOG_FORMAT=json\`."

new_issue "Fix typo in onboarding email" \
  "Implemented,docs" \
  "\"Welcome aboard!\" was spelled \"Wellcome aboard!\"."

new_issue "Rate limit public API endpoints" \
  "Implemented,enhancement" \
  "Token bucket rate limiter: 60 req/min per API key on the free tier. Returns \`429\` with \`Retry-After\` header."

new_issue "Migrate CI from Travis to GitHub Actions" \
  "Implemented" \
  "One workflow per stage (lint, test, build). Matrix across Go 1.24 and 1.25."

echo "==> Done. Run 'lazyboards' in this repo to verify the board."
