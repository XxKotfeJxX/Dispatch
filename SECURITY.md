# Security policy

## Supported versions

The latest tagged release receives security fixes.

## Reporting

Do not open a public issue for a suspected vulnerability. Use GitHub private vulnerability reporting in the repository Security tab.

Include the affected version, reproduction, impact, and suggested mitigation. Remove tokens, addresses, and message content from evidence.

## Deployment baseline

Replace the development API key, terminate TLS at a trusted reverse proxy, restrict network access to PostgreSQL and Mailpit, keep `WEBHOOK_ALLOW_PRIVATE=false`, run containers as non-root, and rotate provider credentials regularly.
