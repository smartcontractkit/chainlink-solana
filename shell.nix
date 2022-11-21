{ stdenv, pkgs, lib }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    (rust-bin.stable.latest.default.override { extensions = ["rust-src"]; })
    lld_10
    llvm_11
    stdenv.cc.cc.lib
    pkg-config
    udev
    openssl

    # Solana
    # solana.solana-full
    # spl-token-cli
    # anchor

    # Golang
    # Keep this golang version in sync with the version in .tool-versions please
    go_1_19
    gopls
    delve
    golangci-lint
    gotools

    # NodeJS + TS
    nodePackages.typescript
    nodePackages.typescript-language-server
    # Keep this nodejs version in sync with the version in .tool-versions please
    nodejs-18_x
    (yarn.override { nodejs = nodejs-18_x; })
    python3
  ] ++ lib.optionals stdenv.isLinux [
    # ledger specific packages
    libudev-zero
    # udev.lib
    libusb1
  ];
  RUST_BACKTRACE = "1";
  # https://github.com/rust-lang/rust/issues/55979
  LD_LIBRARY_PATH = lib.makeLibraryPath (with pkgs; [ stdenv.cc.cc.lib libudev-zero ]);
  GOROOT="${pkgs.go_1_19}/share/go";

  # Avoids issues with delve
  CGO_CPPFLAGS="-U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0";
}
