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

## Sensitive Data
- No sensitive data in logs
- No PII in error responses
- Audit logging for sensitive operations

## Untrusted Data Rendering
- When fixing untrusted-data rendering (sanitizing control bytes, escaping markdown metacharacters, etc.), don't verify only the stated call site — grep for all render sites of the *same data types* (e.g., if fixing `card.Body` rendering in one place, find every place that renders `card.Title`, label names, milestone, PR titles, assignee identifiers, etc.). A fix limited to one render path while siblings remain unsanitized creates false confidence and leaves exploitable gaps.
