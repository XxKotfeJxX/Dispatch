# ADR 0002: AI is advisory analysis

Status: Accepted

Gemini may classify category and priority, create a summary, report confidence, and provide reason codes. It never selects delivery channels.

The recipient selected by an integration is the single source of truth for delivery. Dispatch uses that recipient's configured destination and does not silently reroute a notification to another channel.

Only structured output matching the local schema and confidence threshold is accepted. Disabled AI, timeout, provider errors, invalid output, and low confidence all produce an auditable deterministic fallback. Dispatch remains fully usable without an AI key.
