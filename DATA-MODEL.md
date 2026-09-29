# Data Model & State Ownership

Tribunal uses a strictly normalized SQLite database embedded directly within the Go binary process. This ensures offline reliability, zero network latency for data access, and eliminates the "quietly losing records" problem common in distributed sync systems.

## Schema Overview

The database uses robust relational constraints to maintain integrity across the event lifecycle.

### Core Entities

*   `events`: The root boundary object. Holds `submissions_close` to strictly enforce deadline state at the database level.
*   `users`: Authenticated accounts. Contains a `role` enum (`participant`, `judge`, `organizer`) and a `ref_id` linking to their specific role data.
*   `sessions`: High-entropy session tokens with absolute `expires_at` timestamps.

### Submissions & Teams

*   `teams` and `team_members`: A team has a many-to-many relationship with users (via email), allowing members to be added before they even register.
*   `tracks`: The prize categories.
*   `projects`: The core submission. Tied to a `team_id` and a `track_id`. Contains `submitted_at` to audit deadline compliance.

### Judging & Integrity

*   `judges`: The judge directory.
*   `judge_tracks`: A cross-reference table binding judges to specific tracks, preventing a judge from evaluating outside their domain.
*   `judge_assignments`: The algorithmic assignment tracker. Enforces load balancing, conflict resolution, and ensures sufficient coverage per project. Tracks assignment state (`assigned`, `started`, `completed`).
*   `scores`: The standard rubric scoring. Composite primary key on `(judge_id, project_id)` enforces idempotency (a judge can only score a project once). `criteria` is a JSON blob for flexible rubric structures.
*   `pairwise_comparisons`: The immutable ledger for the Bradley-Terry ranking engine. Records `(judge_id, winner_id, loser_id, created_at)`. Append-only.
*   `audit_log`: The cryptographic hash-chain tracking every significant state mutation (score submissions, edits, assignments) to ensure tamper-evidence and independent verifiability.

## Concurrency and Write-Ahead Logging (WAL)

SQLite is traditionally viewed as a single-user database. We overcome this by enabling **Write-Ahead Logging (WAL)** mode upon initialization:

```sql
PRAGMA journal_mode=WAL;
```

This ensures readers do not block writers and writers do not block readers, providing the necessary concurrency for a room of 50 judges submitting scores simultaneously, without the operational overhead of Postgres.

## Import and Export Paths

Data portability is a core requirement (T2).
*   **Import**: Bootstrapping occurs automatically via `fixtures.json`. When the platform starts, it checks for an empty database and hydrates it atomically within a single transaction.
*   **Export**: The `/api/export.csv` endpoint generates a denormalized, flattened CSV of all projects, teams, and aggregated scores. This ensures organizers have a portable, vendor-neutral copy of the final state.
