# Local Rust security remediation

PREPARED LOCALLY - NOT PUSHED, RELEASED OR INSTALLED.

Rust source d8c68f5534e5aaa9d4f994751bc763e3938c2c22. Tokio1.53.3, Electrum0.24.2 and roots1.0.10; obsolete test auto-download stack removed, externally supplied checksum-verified test binaries retained. Exact-channel source and generated Go/C APIs unchanged. Both macOS15.4 slices rebuilt using Rust1.91.1, locked release/uniffi, API exports verified identical. Universal SHA256 19f9cbea84af10665fe10370a7c1d6e586a2cf8f9cd23a0458ab7e069e411d3f.

Production normal/build dependency graph has no advisory-listed vulnerabilities. Raw audit retains8 entries only in pinned LDK0.6.2 VSS compatibility-test dependency plus5 maintenance warnings; not a clean repository-wide audit. Other-platform native binaries inherited/unvalidated. No wallet operations. No push/publication/install approved.
