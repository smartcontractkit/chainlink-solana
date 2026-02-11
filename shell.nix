{ stdenv, pkgs, lib }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    (rust-bin.stable.latest.default.override { extensions = ["rust-src"]; })
    # lld_11
    llvm_12
    stdenv.cc.cc.lib
    pkg-config
    openssl

    # Solana
    # solana.solana-full
    # spl-token-cli
    # anchor

    # Golang
    # Keep this golang version in sync with the version in .tool-versions please
    go_1_24
    gopls
    delve
    golangci-lint
    gotools

    # NodeJS + TS
    nodePackages.typescript
    nodePackages.typescript-language-server
    nodePackages.npm
    nodePackages.pnpm
    # Keep this nodejs version in sync with the version in .tool-versions please
    nodejs_20
    (yarn.override { nodejs = nodejs_20; })
    python3
    ] ++ lib.optionals stdenv.isLinux [
      # ledger specific packages
      libudev-zero
      libusb1
    ];
  RUST_BACKTRACE = "1";

  LD_LIBRARY_PATH = lib.makeLibraryPath [pkgs.zlib stdenv.cc.cc.lib]; # lib64

  # Avoids issues with delve
  CGO_CPPFLAGS="-U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0";

  shellHook = ''
    # install gotestloghelper
    go install github.com/smartcontractkit/chainlink-testing-framework/tools/gotestloghelper@latest
  '';
}
