# ADR 0002: AI is advisory analysis

Status: Accepted

Gemini may classify category and priority, create a summary, report confidence, and provide reason codes. It never selects delivery channels.

The recipient selected by an integration is the single source of truth for delivery. Dispatch uses that recipient's configured destination and does not silently reroute a notification to another channel.

Only structured output matching the local schema and confidence threshold is accepted. Disabled AI, timeout, provider errors, invalid output, and low confidence all produce an auditable deterministic fallback. Dispatch remains fully usable without an AI key.

Analysis uses a stable taxonomy instead of allowing the model to invent labels:

- categories: `security`, `finance`, `development`, `communication`, `calendar`, `tasks`, `files`, `account`, `system`, `content`, and `general`
- priorities: `low`, `normal`, `high`, and `critical`
- controlled reason codes describe the evidence behind each decision

`critical` is reserved for explicit active compromise, production outage, or immediate irreversible financial loss. Direct messages, mentions, unread state, and unsupported urgency language do not raise priority by themselves.

The Gemini request separates system instructions from untrusted notification JSON, disables response storage, limits input and output size, and uses structured output. Summaries stay factual, use the notification's main language, and never infer attachment contents from a filename.
