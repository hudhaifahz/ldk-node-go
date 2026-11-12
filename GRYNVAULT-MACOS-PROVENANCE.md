# Grynvault macOS native library

Native source: https://github.com/hudhaifahz/ldk-node/commit/e270baac5f6f8bf440a8c80f3365b15f9d911e94
Base: d8c68f5534e5aaa9d4f994751bc763e3938c2c22 (patched TLS and Tokio dependencies).

Build with Rust 1.89.0, the committed Cargo.lock, `MACOSX_DEPLOYMENT_TARGET=15.4`,
and `cargo build --locked --release --features uniffi --target TARGET` for
`aarch64-apple-darwin` and `x86_64-apple-darwin`. Combine both dylibs with
`lipo -create`. Install name remains `@rpath/libldk_node.dylib`.

Universal dylib SHA-256:
`2c1ebe99232a306053c586f8a089a9d6546d034b91a77c0d83248c7333cbe5f2`

All 480 UniFFI exports match the prior macOS library on both architectures.
57 Rust library tests passed. The Go API/UDL is unchanged. Other platform
libraries are unchanged and do not include this macOS release repair.

The native patch waits for opt-in local SOCKS transport before starting the
Lightning handshake, bounds reconnect delays, connects peers independently,
and clears pending state on cancellation. No wallet data or peer configuration
is included in this repository.
