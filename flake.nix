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
        # import solana packages
        solanaPkgs = pkgs.callPackage ./solana.nix {};
      in {
        formatter = pkgs.nixpkgs-fmt;

        devShells = {
          default = pkgs.callPackage ./shell.nix {
            inherit pkgs;
            scriptDir = toString ./.; # converts the flakes'root dir to string
          };
          # interactive shell with Solana CLI tool accessibility
          solana-cli = solanaPkgs.solana-cli-shell;
        };

        packages = {
          # dockerized Solana test validator
          solana-test-validator = solanaPkgs.solana-test-validator;
          # environment package for Solana CLI
          solana-cli-env = solanaPkgs.solana-cli-env;
        };
    });
}
