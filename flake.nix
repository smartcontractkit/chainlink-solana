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
      in {
        devShell = pkgs.callPackage ./shell.nix {
          inherit pkgs;
          scriptDir = toString ./.; # converts the flakes'root dir to string
        };

        packages = {
          solana-test-validator = pkgs.stdenv.mkDerivation rec {
            name = "solana-test-validator";
            src = ./scripts; 
            installPhase = ''
              mkdir -p $out/bin
              cp $src/setup-test-validator/localnet.sh $out/bin/${name}
              cp $src/setup-test-validator/localnet.down.sh $out/bin/
              chmod +x $out/bin/${name}
            '';
          };
        };
    });
}
