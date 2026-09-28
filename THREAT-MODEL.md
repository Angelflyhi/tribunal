# Threat Model

A hackathon platform is fundamentally a system of governance and resource distribution (prizes). It is susceptible to gaming, abuse, and collusion. This document outlines the threats we mitigated and those we accepted.

## Mitigated Threats

### 1. Insecure Direct Object Reference (IDOR) & Peer Isolation
**Threat:** A judge modifies the API request to view or edit another judge's scores, potentially leading to collusion or intimidation.
**Mitigation:** The `authMiddleware` injects a secure `userContextKey` into the request context. The `/api/judge/scores` endpoint pulls the `RefID` (Judge ID) directly from the secure server-side session context, not from the URL parameters or body payload. It is impossible for a judge to query scores for a `RefID` they do not own.

### 2. Sybil Voting & Ballot Stuffing
**Threat:** Participants create multiple fake accounts to inflate their own project's score.
**Mitigation:** We explicitly reject the "community voting" model. Scores and pairwise comparisons are strictly limited to accounts with the `judge` role. Judge accounts cannot be self-registered; they are either seeded by the organizer via fixtures or created by an authenticated `organizer` session.

### 3. Submission Deadline Gaming
**Threat:** A team submits or modifies their project after the `submissions_close` deadline to gain extra time.
**Mitigation:** The `POST /projects/new` endpoint performs a strict time check against the `events.submissions_close` timestamp before inserting the record. Changes submitted after this timestamp result in a hard 403 Forbidden.

### 4. SQL Injection
**Threat:** Malicious input in form fields compromises the database.
**Mitigation:** We use `database/sql` parameterization (`?` placeholders) exclusively for all dynamic queries. We do not use ORMs that might introduce complex side-effects, nor do we construct raw SQL strings via concatenation.

## Accepted Risks

### 1. Physical Device Compromise
Because this platform is designed to be run offline on a laptop (One Command to Running), the database file (`dogfood.sqlite`) is stored locally. An attacker with physical access to the organizer's laptop can directly modify the SQLite database, bypassing the application layer entirely.

### 2. Denial of Service (DoS)
As an offline-first system designed for LAN or localized deployments, we do not implement complex rate-limiting or DDoS protection. If a malicious participant spams the local network, the single Go binary might exhaust its connections. We accept this risk as deploying Cloudflare or similar proxies violates the "offline-first, no hosted service dependency" requirement.
