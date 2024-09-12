{
  description = "Solana integration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay.url = "github:oxalica/rust-overlay";
  };

  outputs = inputs@{ self, nixpkgs, rust-overlay, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; overlays = [ rust-overlay.overlays.default ]; };
        solanaPkgs = pkgs.callPackage ./solana.nix {};
      in rec {
        devShells = {
          default = pkgs.callPackage ./shell.nix {};
          solana-cli = solanaPkgs.solana-cli-shell;
        };

        packages = {
          solana-test-validator = solanaPkgs.solana-test-validator;
          solana-cli-env = solanaPkgs.solana-cli-env;
        };

        apps.solana-build-programs = flake-utils.lib.mkApp {
          drv = solanaPkgs.solana-build-programs;
        };
    });
}
