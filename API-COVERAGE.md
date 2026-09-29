# Tribunal API Coverage (Dogfood 2026)

This document maps all required endpoints for the Dogfood 2026 spec and their implementation status in Tribunal.

## T1: Core Functionality
- `POST /projects/new`: Submit project - **Implemented**
- `PUT /api/projects/{id}`: Update project - **Implemented**
- `DELETE /api/projects/{id}`: Delete project - **Implemented**
- `GET /api/judge/scores`: Get judging scores - **Implemented**

## T2: Organizer & Audit Tools
- `GET /api/export.csv`: Export projects and teams to CSV - **Implemented**
- `GET /api/audit`: Cryptographic hash-chained audit log of critical actions - **Implemented**
- `GET /api/assignments`: Get judge assignments - **Implemented**
- `GET /api/judging/progress`: Get judging progress and statistics - **Implemented**
- `POST /api/import`: Bulk import projects and users (fixtures format) - **Implemented**

## T3: Public Surface & Community
- `GET /embed/gallery`: Embeddable gallery for external sites - **Implemented**
- `POST /api/projects/{id}/vote`: Public community voting with multiple modes (authenticated, email-gated, quadratic) - **Implemented**
- `POST /api/projects/{id}/comment`: Submit a public comment on a project - **Implemented**
- `GET /results`: Leaderboard results, hidden until configurable voting close time passes - **Implemented**

## T4: Complete Chain & Offline Verification
- `GET /api/export/bundle`: Downloads the signed results bundle (`results.json`, `audit-anchor.json`, `manifest.json`) - **Implemented**
- `GET /api/export/lossless`: Exports the raw SQLite database for a lossless snapshot - **Implemented**
- `POST /api/import/lossless`: Imports a raw SQLite database and restarts Tribunal with the new state - **Implemented**
- `GET /api/certificate`: Generates a cryptographically signed participant certificate - **Implemented**

## Additional Endpoints
- `POST /judge/pairwise`: Advanced Bradley-Terry model pairwise comparison (Adaptive pairing) - **Implemented**
- `POST /api/webhooks`: Delivery endpoint for event simulation (e.g., project submission webhooks) - **Implemented**
