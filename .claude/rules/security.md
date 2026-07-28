# Security Rules

## Endpoint Security
- Input validation on all endpoints
- Authorization checks on all endpoints
- No secrets or credentials in code
- No stack traces in user-facing error responses

## Injection Prevention
- Parameterized queries (or ORM) for all database access
- No raw string concatenation for SQL
- Sanitize user input before rendering
- When validating single-rune input from untrusted sources (config YAML, user input) with `utf8.DecodeRuneInString`, check for the `RuneError` sentinel with `size == 1` **before** calling `unicode.IsPrint()` — `IsPrint(utf8.RuneError)` unexpectedly returns true (U+FFFD REPLACEMENT CHARACTER is category So), which can cause invalid byte sequences like `"\xff"` to be silently accepted as valid keys. Additionally, reject format-category runes (`unicode.Cf` category: zero-width joiners/spaces, bidi overrides, etc.) in addition to non-printable ones, as these pose bidi-spoofing/control-sequence injection risks even though IsPrint would accept them

## Sensitive Data
- No sensitive data in logs
- No PII in error responses
- Audit logging for sensitive operations

## Untrusted Data Rendering
- When fixing untrusted-data rendering (sanitizing control bytes, escaping markdown metacharacters, etc.), don't verify only the stated call site — grep for all render sites of the *same data types* (e.g., if fixing `card.Body` rendering in one place, find every place that renders `card.Title`, label names, milestone, PR titles, assignee identifiers, etc.). A fix limited to one render path while siblings remain unsanitized creates false confidence and leaves exploitable gaps.
