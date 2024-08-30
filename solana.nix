{
  system ? builtins.currentSystem,
  pkgs ? import <nixpkgs> { inherit system; },
}:

let
  version = "v1.18.22";
  getBinDerivation = { name, filename, sha256 }:
    pkgs.stdenv.mkDerivation rec {
      inherit name;
      url = "https://github.com/anza-xyz/agave/releases/download/${version}/${filename}";
      src = pkgs.fetchzip {
        inherit url sha256;
      };

      installPhase = ''
        mkdir -p $out/bin
        ls -lah $src
        cp -r $src/bin/* $out/bin
      '';
    };
in
{
  # Provides Solana CLI tool accessibility.
  # It provides two derivations, one for x86_64-linux and another for aarch64-apple-darwin.
  # Each derivation downloads the corresponding Solana release.
  binaries = {
    x86_64-linux = getBinDerivation {
      name = "solana-cli-x86_64-linux";
      filename = "solana-release-x86_64-unknown-linux-gnu.tar.bz2";
      sha256 = "sha256-bgT7Xqnz6V8tsv5WSESbSUfJCPfGWjGHGYvpEG0myxk=";
    };
    aarch64-apple-darwin = getBinDerivation {
      name = "solana-cli-aarch64-apple-darwin";
      filename = "solana-release-aarch64-apple-darwin.tar.bz2";
      sha256 = "sha256-eqJcoheUCACcIfNNgMGhbhYnAyAy9PGarlWhzr4JpbU=";
    };
  };
  shellHook = ''
    echo "===================================================="
    echo "Welcome to the Solana CLI dev shell."
    echo "Current environment: $(uname -a)"
    echo "You are using the package for ${pkgs.stdenv.hostPlatform.system}."
    echo "----------------------------------------------------"
    echo "Solana CLI information:"
    solana --version
    solana config get
    echo "===================================================="
  '';

  # Package: Provides dockerized Solana test validator accessibility.
  # https://hub.docker.com/r/solanalabs/solana/
  # Currently only supports x86_64-linux.(https://github.com/anza-xyz/agave/tree/master/sdk/docker-solana)
  solana-test-validator = pkgs.stdenv.mkDerivation rec {
    name = "solana-test-validator";
    src = ./scripts/setup-localnet; 
    installPhase = ''
      mkdir -p $out/bin
      cp $src/localnet.sh $out/bin/${name}
      cp $src/localnet.down.sh $out/bin/
      cp $src/get-latest-validator-release-version.sh $out/bin/
      chmod +x $out/bin/${name}
    '';
  };
}