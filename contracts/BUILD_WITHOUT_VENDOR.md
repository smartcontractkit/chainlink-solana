# Building Without the vendor/ Directory

The default Docker build (Rust 1.79 in `backpackapp/build:v0.31.0`) requires vendored crates because:

- **constant_time_eq** 0.4.x from crates.io needs Cargo edition2024 (Rust 1.85+).
- **zmij** uses `core::hint::select_unpredictable`, which is only in Rust 1.88+.

To avoid `vendor/` and use crates.io only:

1. **Use a build image with Rust 1.88+** (and ideally Cargo that supports edition2024, e.g. Rust 1.85+), so that:
   - zmij works as-is (its `build.rs` enables the fallback only when `rustc < 88`).
   - constant_time_eq 0.4.x and blake3 1.8.3+ can be used from crates.io.

2. **Remove the patches and script hacks:**
   - In `contracts/Cargo.toml`: delete the entire `[patch.crates-io]` block.
   - In `scripts/anchor-build.sh`: remove the `awk` block that rewrites `Cargo.lock` (constant_time_eq, zmij, blake3) and the `rm -rf` cache cleanup for those crates (or run with `ANCHOR_BUILD_NO_VENDOR=1` if you added that support).

3. **Refresh the lockfile:**
   ```bash
   cd contracts && cargo update
   ```

4. **Remove the vendor directory:**
   ```bash
   rm -rf contracts/vendor/constant_time_eq contracts/vendor/zmij
   ```
   (Leave `vendor/` if other vendored crates remain.)

5. **Optional:** In `programs/keystone-forwarder/Cargo.toml` you can relax the zmij pin to `zmij = "1.0"` if the image has Rust 1.88+.

If no official Anchor/Solana image with Rust 1.88+ exists yet, you can build your own Dockerfile on top of a newer Rust image and install Anchor + Solana tools there, then point `PROJECT_SERUM_IMAGE` (or your build script) at that image.
