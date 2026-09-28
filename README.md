# Tribunal: The Hackathon Judging Platform

Tribunal is a self-hostable, end-to-end hackathon judging and submission platform. It was built for the **Dogfood 2026** hackathon.

It differentiates itself by using **statistical rigor** in the judging process. Rather than relying on naive averages which are easily skewed by harsh or lenient judges, Tribunal implements:
1. **Bradley-Terry Pairwise Comparisons** via MM-algorithm to ensure monotonic convergence of project qualities.
2. **Item Response Theory (IRT)** for judge calibration, automatically discounting scores from inconsistent or strictly harsh judges.
3. **Z-Score Normalization** across all judge rubrics.
4. **Bootstrap Confidence Intervals** to mathematically prove the final ranking to sponsors and participants.

## Running the Platform

To run Tribunal locally with a single command (as per the spec):
```bash
docker-compose up
```

This will:
- Build the Go backend from source.
- Start an Alpine-based container.
- Map the internal SQLite DB to your `./data` directory.
- Load the initial `fixtures.json` (if present).
- Expose the platform on `http://localhost:8080`.

## Features & Compliance
- ✅ Gallery browsing (T1)
- ✅ Submission blocking after event close (T1)
- ✅ Configurable Weighted Scoring Rubric (T2)
- ✅ Strict Role & Peer Isolation (T2)
- ✅ Organizer CSV export (T2)
- 🚀 **Advanced Pairwise Mode (T3 Bonus)**
- 🚀 **Normalization Proof (T4 Bonus)** - See `JUDGING.md` for mathematical proofs.
- 🛡️ **Security Threat Model & RBAC** - See `THREAT-MODEL.md` for our zero-trust implementation details.
- 🔌 **API-First Design** - See `openapi.yaml` for our OpenAPI 3.0 specification mapping all UI interactions to JSON endpoints.

## Implementation & Licensing
Built with Go 1.22 and `modernc.org/sqlite` (CGO-free SQLite). No external dependencies. No C-toolchain required.
Released as Open Source Software. Review the code to see our implementation of the MM-Algorithm.
