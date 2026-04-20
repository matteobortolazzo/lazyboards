# demo.tape — VHS recipe for the README GIF
# Requires: vhs, ttyd, ffmpeg (pacman -S vhs ttyd ffmpeg)
# Regenerate with: vhs demo.tape

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
Type "cd ~/Repos/lazyboards-demo" Enter
Type "clear" Enter
Show

# ---- Launch ----
Type "lazyboards" Enter
Sleep 2500ms

# ---- Navigate columns ----
Tab
Sleep 700ms
Tab
Sleep 700ms
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
