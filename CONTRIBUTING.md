# Contributing

Open an issue before a large change. Keep changes within the documented single-tenant scope and include tests for routing, queue, delivery, or security behavior.

1. Create a focused branch from `main`.
2. Run `go test ./...` and `cd web && pnpm test && pnpm build`.
3. Use a Conventional Commit (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`).
4. Open a pull request using the repository template.

Never commit `.env`, API keys, notification payloads from real users, or provider responses containing sensitive data.
