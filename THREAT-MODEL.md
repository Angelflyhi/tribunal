# Tribunal Threat Model

Tribunal is designed on a zero-trust model where we assume participants may attempt to exploit the platform to manipulate rankings, and that even the internal database could be exposed to tampering by malicious insiders.

## Scope of Protection

1. **Role Bypass & Privilege Escalation**
2. **Ballot Stuffing & Vote Manipulation**
3. **Audit Log Tampering**
4. **Data Leakage (Pre-Closing)**

## Threat Scenarios & Mitigations

### 1. Privilege Escalation
**Threat:** A participant attempts to access the organizer dashboard or export endpoints.
**Mitigation:** Strict backend enforcement. The `authMiddleware` injects the `User` object into the request context. Every protected endpoint rigorously checks `user.Role == "organizer"`. There is no client-side trust.

### 2. Ballot Stuffing
**Threat:** A user writes a script to spam the public `/api/projects/{id}/vote` endpoint.
**Mitigation:** 
- Configurable **Rate Limiting** (60 requests per IP per minute) via `rateLimitMiddleware`.
- **Identity Gating**: Votes are tracked by IP address or Email (in authenticated modes). SQLite enforces `UNIQUE(project_id, user_email)` or `UNIQUE(project_id, ip_address)` preventing double voting at the database level.

### 3. Database Tampering (The Inside Job)
**Threat:** A malicious organizer with direct SSH access to the server manually runs `UPDATE projects SET score = 9999` in the SQLite file.
**Mitigation:** The **Cryptographic Audit Chain**.
Every sensitive action (judge assignment, voting, score updates) generates a record in the `audit_log` table.
- Each record contains a SHA-256 hash calculated as `HASH(prev_hash + event_id + actor_id + action + payload + timestamp)`.
- The `tribunal doctor` command actively verifies this chain. If an attacker modifies past data, the hash chain breaks, immediately flagging the database as tampered.

### 4. Premature Result Leakage
**Threat:** Participants try to view the leaderboard before the hackathon ends to gain an unfair advantage.
**Mitigation:** The `/results` endpoint reads the `voting_close` timestamp from the `events` table. If the current time is before the deadline, the endpoint strictly returns a `403 Forbidden` error and blocks all data transfer.

## T4 Integrity Assurances
For external judges to trust the system, they don't need access to our server. We generate a **Signed Results Bundle** (`results-bundle.zip`).
This bundle contains:
1. `results.json`: The final leaderboard.
2. `audit-anchor.json`: The terminal hash of the audit log chain.
3. `manifest.json`: An HMAC-SHA256 signature of the internal files using a server secret.

Anyone can use `tribunal verify-results bundle.zip` to prove cryptographically that the exported results match exactly what the server produced, without needing to trust the transmission medium.
