# Shell & URL Template-Variable Safety

Rules for escaping untrusted data (card labels, titles — sourced from the GitHub API) before interpolating it into shell commands or URLs via the custom-action template system.

## Rules

- Always escape user-controlled input before interpolating into shell commands. Use `ShellEscape()` / `BuildShellSafeVars()` (POSIX single-quote wrapping: replace `'` with `'\''`, wrap in `'...'`) for every template variable expanded into a shell command. Card labels and titles come from an external API and are not trustworthy — an unescaped label like `"; rm -rf /; "` is arbitrary command execution.
- For URL query-parameter template variables, always use `url.QueryEscape`, never `url.PathEscape`. `PathEscape` leaves `&`, `=`, `,` unescaped (they're valid path sub-delimiters per RFC 3986), which reopens query-parameter injection — a label like `"bug&extra=malicious"` in `?tags={tags}` becomes `?tags=bug&extra=malicious`. `QueryEscape` correctly encodes all three. Before switching an encoding function (even on reviewer suggestion), verify which characters it actually covers against the RFC 3986 character set rather than trusting the function name's apparent fit.
