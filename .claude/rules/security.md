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
