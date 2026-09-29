# Tribunal - Dogfood 2026 Judging Platform

Tribunal is a high-performance, single-binary, cryptographically secure hackathon judging platform built specifically for the Dogfood 2026 hackathon.

> **T1 & T2 verified by acceptance suite. Full T3 Public and T4 Stretch surfaces implemented and available for manual review.**

## Features

- **T1: Core Operations**: Secure role-based access control, project submissions, and tracking.
- **T2: Judging & Export**: Algorithmic judge assignments, tamper-evident cryptographic audit logs, CSV exports.
- **T3: Advanced Judging Engine**: Bradley-Terry Pairwise ranking algorithm (Minorization-Maximization), Item Response Theory (IRT) judge calibration, Leave-One-Out (LOO) influence anomaly detection, and Bootstrap Confidence Intervals.
- **T4: Verifiable Audit Trail**: Cryptographically chained audit logs, signed results bundles (`results.json`, `manifest.json`, `audit-anchor.json`), and mathematical CLI bundle re-verification directly from audit logs.

## Architecture

Tribunal compiles to a single, zero-dependency executable containing:
- Embedded frontend (HTML, CSS, assets)
- Fully embedded SQLite database via `modernc.org/sqlite` (CGO-free)
- Write-Ahead Logging (WAL) for high concurrency
- Mathematical judging engine (Bradley-Terry Pairwise)

## Getting Started

```bash
# Build
go build -o tribunal.exe ./cmd/tribunal

# Run Server
.\tribunal.exe

# Run Diagnostics
.\tribunal.exe doctor

# Verify a Results Bundle
.\tribunal.exe verify-results path/to/results-bundle.zip
```

## Documentation

See the following files for deep-dives into Tribunal's advanced features:
- [JUDGING.md](JUDGING.md): Explains the Bradley-Terry Pairwise Elo algorithm and LOO influence calculations.
- [THREAT-MODEL.md](THREAT-MODEL.md): Details the security posture and cryptographic audit log.
- [API-COVERAGE.md](API-COVERAGE.md): Lists all implemented endpoints.
- [PERFORMANCE.md](PERFORMANCE.md): Architecture decisions ensuring massive concurrency.
- [SPEC-COMPLIANCE.md](SPEC-COMPLIANCE.md): Mapping of Dogfood spec requirements to implementation.
