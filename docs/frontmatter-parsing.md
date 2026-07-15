# Frontmatter Delimiter Parsing

`parseFrontmatter` (`commands.go`) splits the composed `"---\ntitle: TITLE\n---\nBODY"` string produced by the edit-mode frontmatter composer back into title, labels, and body.

## Rules

- Never split on a bare `"---"` substring (e.g. `strings.SplitN(content, "---", 3)`). A title containing `---` (e.g. "My --- Title") splits at the wrong position and silently corrupts the parsed title/body. Match the exact delimiter format the composer emits instead: find the closing delimiter via `strings.Index(content, "\n---\n")`, with a `strings.HasSuffix(content, "\n---")` fallback for the EOF case (no trailing newline after the closing delimiter).
