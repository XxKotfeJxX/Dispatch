# ADR 0002: AI routing is advisory

Status: Accepted

Gemini may classify category, priority, summary, channels, urgency, confidence, and reason codes. It cannot bypass explicit routes, deterministic rules, recipient preferences, provider availability, or security policy.

Only structured output matching the local schema and confidence threshold is accepted. Disabled AI, timeout, provider errors, invalid output, and low confidence all produce an auditable deterministic fallback. Dispatch remains fully usable without an AI key.
