{
  system ? builtins.currentSystem,
  pkgs ? import <nixpkgs> { inherit system; },
}:

# It provides two derivations, one for x86_64-linux and another for aarch64-apple-darwin.
# Each derivation downloads the corresponding Solana release.
let
  version = "v1.99.22";
  getBinDerivation =
    {
      name,
      filename,
      sha256,
    }:
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

  solanaBinaries = {
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
in
{
  # Provides environment package for Solana CLI
  solana-cli-env = pkgs.buildEnv {
    name = "solana-cli-env";
    paths = pkgs.lib.optionals pkgs.stdenv.isLinux [
      solanaBinaries.x86_64-linux
    ]
    ++ pkgs.lib.optionals (pkgs.stdenv.isDarwin && pkgs.stdenv.hostPlatform.isAarch64) [
      solanaBinaries.aarch64-apple-darwin
    ];
  };

  # Provides interactive shell with Solana CLI tool accessibility.
  solana-cli-shell = pkgs.mkShell {
    buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
      solanaBinaries.x86_64-linux
    ]
    ++ pkgs.lib.optionals (pkgs.stdenv.isDarwin && pkgs.stdenv.hostPlatform.isAarch64) [
      solanaBinaries.aarch64-apple-darwin
    ];
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
  };

  # Provides dockerized Solana test validator accessibility.
  # https://hub.docker.com/r/solanalabs/solana/
  # Currently the official docker image only supports x86_64-linux.(https://github.com/anza-xyz/agave/tree/master/sdk/docker-solana)
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