# Build requirements

Contract build uses crates.io only (no vendored crates). It requires a build image with **Rust 1.88+** so that:

- **zmij** works (its `build.rs` uses `select_unpredictable` only when `rustc >= 88`).
- **constant_time_eq** 0.4.x and **blake3** 1.8.3+ work (edition2024 / Rust 1.85+).

Use a Docker image that ships with this toolchain (e.g. a newer `backpackapp/build` tag or a custom image with Anchor + Solana tools on a recent Rust).
