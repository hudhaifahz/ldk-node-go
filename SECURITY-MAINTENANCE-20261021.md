# Local TLS security maintenance candidate

PREPARED LOCALLY — NOT PUSHED, RELEASED, OR INSTALLED.

- Source: `22c66d60ffe13e6dd0532d7f22501aa55b6a769f` in the sibling `ldk-node` worktree.
- Preserves reviewed exact-channel source `70965f2e58936c0893c48ee0cc1788b84164a4e2` unchanged.
- Replays upstream TLS mitigation `87ea53f2e1f32cc7487521776ae10f9e3850a891`: rustls-webpki 0.103.16 and locked native builds.
- Native build: Rust 1.91.1, `cargo build --locked --release --features uniffi`, arm64 and x86_64 Apple targets, `MACOSX_DEPLOYMENT_TARGET=15.4`, combined with `lipo`.
- Universal library SHA-256: `bb487c9c2640e4c053aa508998f459b6b03216be4592ea080a64a4e9b5476f78`. Install name remains `@rpath/libldk_node.dylib`.
- Generated Go/C API files are inherited unchanged; no exact-channel APIs removed.
- macOS is the supported fork target. Other platform libraries are inherited and have NOT been rebuilt with this mitigation or validated for exact-channel use.
- Raw Rust audit remains nonzero: root-store advisory RUSTSEC-2026-0381 remains visible in production roots 0.25.4/1.0.9 despite the fixed intermediate-SPKI verifier; other flagged versions are test/build-only. No clean audit or complete TLS-stack acceptance claim.
- Owner approval is required before pushing this branch or publishing any module, tag, signature, release, or installation.
