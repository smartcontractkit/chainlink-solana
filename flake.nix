{
  description = "Solana integration";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay.url = "github:oxalica/rust-overlay";
    # saber-overlay.url = "github:saber-hq/saber-overlay";
    # saber-overlay.inputs.rust-overlay.follows = "rust-overlay";
    # naersk.url = "github:nmattia/naersk";
  };

  outputs = inputs@{ self, nixpkgs, rust-overlay, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; overlays = [ rust-overlay.overlays.default ]; };
        # naerskLib = pkgs.callPackage naersk {
        #   inherit (pkgs.rust-bin.nightly.latest) rustc cargo;
        # };
      in rec {
        # packages.program = naerskLib.buildPackage {
        #   pname = "program";
        #   root = ./.;
        # };
        # defaultPackage = packages.program;
        devShell = pkgs.callPackage ./shell.nix {};
      });
}
