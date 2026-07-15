# demo.tape — VHS recipe for the README GIF
# Requires: vhs, ttyd, ffmpeg (pacman -S vhs ttyd ffmpeg) and python3
# (for the fake cenci socket that drives the agent badges).
# Regenerate with: vhs demo.tape   (from the repo root)

Output docs/demo.gif

Set Shell "zsh"
Set FontSize 14
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms
Set Framerate 30

# ---- Silent setup ----
Hide
# Fake cenci daemon in a throwaway runtime dir: scripted snapshots give
# the demo deterministic agent badges (▶/✓/!) and the ⟳ dispatch segment.
Type "export XDG_RUNTIME_DIR=$(mktemp -d)" Enter
Type "python3 docs/demo-cenci-fake.py &" Enter
Type "cd ~/Repos/lazyboards-demo" Enter
# Pin the demo columns to the labels seeded by docs/demo-repo-seed.sh, so the
# recording is deterministic regardless of any personal global config.
Type "printf 'columns:\n  - name: New\n  - name: Refined\n  - name: Implementing\n' > .lazyboards.yml" Enter
Type "clear" Enter
Show

# ---- Launch ----
Type "lazyboards" Enter
Sleep 2500ms

# ---- Navigate columns (Refined has a ✓ agent badge, Implementing ▶ and !) ----
Tab
Sleep 1s
Tab
Sleep 1200ms
Type "j"
Sleep 800ms
Tab
Sleep 700ms

# ---- Navigate cards within a column ----
Type "j"
Sleep 350ms
Type "j"
Sleep 350ms
Type "j"
Sleep 600ms
Type "k"
Sleep 400ms

# ---- Open detail panel (markdown rendering) ----
Type "l"
Sleep 2s

# ---- Scroll body ----
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 800ms

# ---- Back to card list ----
Escape
Sleep 600ms

# ---- Show help popup ----
Type "?"
Sleep 2500ms
Escape
Sleep 500ms

# ---- Filter flow ----
Type "f"
Sleep 1500ms
Escape
Sleep 500ms

# ---- Git menu ----
Type "g"
Sleep 2s
Escape
Sleep 500ms

# ---- Create card (cancel at the end — don't pollute the demo repo) ----
Type "n"
Sleep 1200ms
Type "Improve onboarding docs"
Sleep 800ms
Tab
Sleep 500ms
Type "docs"
Sleep 800ms
Escape
Sleep 600ms

# ---- Quit ----
Type "q"
Sleep 800ms
