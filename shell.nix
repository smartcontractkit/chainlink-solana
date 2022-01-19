{ stdenv, pkgs, lib }:

pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    (rust-bin.stable.latest.default.override { extensions = ["rust-src"]; })
    lld_10
    llvm_11
    stdenv.cc.cc.lib
    pkg-config
    libudev
    openssl

    # Solana
    solana.solana-full
    spl-token-cli
    anchor

    # Golang
    go_1_17
    gopls
    delve
    golangci-lint

    # NodeJS + TS
    nodePackages.typescript
    nodePackages.typescript-language-server
    nodejs-14_x
    (yarn.override { nodejs = nodejs-14_x; })
  ];
  RUST_BACKTRACE = "1";
  # https://github.com/rust-lang/rust/issues/55979
  LD_LIBRARY_PATH = lib.makeLibraryPath (with pkgs; [ stdenv.cc.cc.lib ]);
  GOROOT="${pkgs.go_1_17}/share/go";
  
  # Avoids issues with delve
  CGO_CPPFLAGS="-U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0";
}
