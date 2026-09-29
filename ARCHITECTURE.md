# Architecture & Operational Readiness

Tribunal is designed to be the ultimate offline-first, highly-available, and dependency-free hackathon platform. In chaotic hackathon environments (bad Wi-Fi, physical infrastructure failures), you must own your state. 

## Core Philosophy: The Single Binary & Zero Dependencies

Hackathons operate in unpredictable environments. The platform cannot rely on complex microservices, Kubernetes clusters, or cloud databases that might go down during judging.

We built Tribunal as a **Single Go Binary** that statically compiles everything it needs into a single executable, including the SQLite database engine (using a CGO-free driver). This provides a true "One command to running" experience, drastically reducing mean-time-to-recovery (MTTR) if the host machine goes down.

### Technology Stack

1.  **Backend:** Go (`net/http`)
    *   No web frameworks. Pure standard library for maximum stability and speed.
    *   Custom session middleware for secure RBAC (Role-Based Access Control).
2.  **Database:** SQLite (`modernc.org/sqlite`)
    *   Embedded database. Data is stored in a single `dogfood.sqlite` file.
    *   No CGO bindings required, meaning the binary can be easily cross-compiled for any OS without a C toolchain.
3.  **Frontend:** HTML5 Templates + Vanilla CSS
    *   Zero frontend build step. No Node.js, no Webpack, no React.
    *   Templates are embedded directly into the Go binary using `go:embed`.
    *   No external CDNs used for fonts or styles to guarantee 100% offline capability.

## Data Model

Our schema is normalized to prevent anomalies and support the complex relationships of a hackathon:
*   `events`: Controls submission deadlines and phases.
*   `users`: Authenticated accounts (bcrypt hashed passwords).
*   `sessions`: Secure session tokens.
*   `teams`: Groupings of users.
*   `projects`: Submissions tied to teams and tracks.
*   `tracks`: Prize categories.
*   `scores`: Standard rubric scoring.
*   `pairwise_comparisons`: Advanced Bradley-Terry A/B testing records.
*   `judge_tracks`: Mapping of which judges evaluate which tracks.
*   `judge_assignments`: Engine enforcing load balancing, conflict-free pairing, and sufficient project coverage.
*   `audit_log`: Cryptographic hash-chain linking all core actions for immutable tamper-evident logs.

## Security (T2 Peer Isolation)

A core requirement was ensuring judges cannot see peer scores, mitigating collusion.
Our data layer enforces strict contextual bounds. The API endpoint `GET /api/judge/scores` verifies the session's role and `RefID` before querying the database, instantly returning `403 Forbidden` if a judge attempts to spoof another judge's ID.

## Mathematical Judging Engine (T3/T4)

We implemented an advanced, state-of-the-art judging engine within the Go binary itself (no external Python microservices needed):
1.  **Z-Score Normalization:** Standardizes standard rubric scores on the fly.
2.  **Bradley-Terry Model:** Uses a Minorize-Maximization (MM) algorithm to iteratively rank projects based on Pairwise Comparisons.

## Extended Features (T3 & T4)
We have fully implemented the T3 Public Voting features and T4 API Extensions for manual review:
- **Public Voting (T3):** `POST /api/projects/{id}/vote` supports open-link, email-gated, and authenticated voting modes with rate limiting and duplicate detection (409 Conflict).
- **Public Results (T3):** `GET /results` enforces a `voting_close` timestamp check, hiding results until judging is finished.
- **Auditing (T3):** A comprehensive `audit_logs` table tracks all actions.
- **REST Expansion (T4):** We expose `PUT` and `DELETE` endpoints for project management.
- **Webhooks & Embeds (T4):** Background goroutines handle real webhook deliveries from DB configurations, and `/embed/gallery` provides a CORS-enabled iframe gallery.
- **Bulk Import (T4):** `POST /api/import` accepts bulk JSON imports using SQL transactions.

## Deployment

The entire system is containerized in a single `Dockerfile` using a multi-stage build.
```bash
docker-compose up --build
```
This single command compiles the binary, sets up the volume for the SQLite database to persist across restarts, and exposes the application on port 8080.
