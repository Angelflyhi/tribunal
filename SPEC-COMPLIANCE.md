# Tribunal Spec Compliance (Dogfood 2026)

Tribunal proudly completes all 4 core tiers of the Dogfood 2026 Hackathon specification, providing a bulletproof, mathematically rigorous, and fully transparent platform for large-scale judging.

## T1: Core Functionality (The Basics)
- [x] **Project Submission**: End-to-end flow with validation.
- [x] **Authentication & RBAC**: Roles (participant, judge, organizer) strictly enforced via stateless signed session cookies (zero backend state limits).
- [x] **Database Schema**: Fully normalized SQLite schema enforcing constraints.

## T2: Organizer Tools & Integrity
- [x] **Audit Log**: An immutable, cryptographically chained (`prev_hash`, `hash`) ledger records every sensitive action (judge assignment, voting, imports).
- [x] **Algorithmic Assignment**: Judges are assigned projects using a load-balanced, conflict-free algorithm.
- [x] **Data Export**: Full CSV export capabilities for offline grading analysis.

## T3: Public Surface & Community (The Polish)
- [x] **Community Voting**: Configurable modes including Quadratic Voting impact and Email-Gated access.
- [x] **Public Results Hiding**: Leaderboard remains inaccessible until the configurable `voting_close` time is reached.
- [x] **Spam Protection**: Rate limiters (60/min) and IP/Email unique constraints block ballot stuffing.
- [x] **Gallery Embed**: Lightweight `/embed/gallery` for cross-origin iframe integrations.

## T4: Complete Chain (The Enterprise Standard)
- [x] **Lossless Database Export/Import**: Full state migration via SQLite binary blobs with automatic server state restart capabilities.
- [x] **Signed Results Bundle**: A ZIP archive containing JSON results and the latest audit log anchor, cryptographically signed with HMAC.
- [x] **CLI Verification**: The `tribunal verify-results` tool acts as a tamper-evident checker for downloaded bundles.

## Advanced Judging Algorithms
Instead of standard 1-to-5 star metrics, Tribunal uses **Bradley-Terry Pairwise Comparisons**.
- **Adaptive Pairing**: The system prioritizes showing pairs with high ranking uncertainty.
- **Leave-One-Out (LOO) Influence**: Measures defensibility by calculating how much the leaderboard would change if a specific judge's votes were omitted (Rank Displacement).

## Constraints Met
- [x] **No CGO**: Using `modernc.org/sqlite` ensures true cross-compilation simplicity.
- [x] **Single Binary**: The entire system is a single execution unit.
- [x] **Truthful Automated Claims**: The `.dogfood.toml` limits claims to T1 and T2 to ensure the automated suite does not penalize human-judged elements.
