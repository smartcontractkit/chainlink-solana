{
  system ? builtins.currentSystem,
  pkgs ? import <nixpkgs> { inherit system; },
}:

# Solana integration
let
  version = "v2.0.8";
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

  # It provides two derivations, one for x86_64-linux and another for aarch64-apple-darwin.
  # Each derivation downloads the corresponding Solana release.

  # The SHA256 hashes below are automatically updated by action.(dependency-updates.yml)
  # The update script(./scripts/update-solana-nix-hashes.sh) looks for the BEGIN and END markers to locate the lines to modify.
  # Do not modify these markers or the lines between them manually.
  solanaBinaries = {
    x86_64-linux = getBinDerivation {
      name = "solana-cli-x86_64-linux";
      filename = "solana-release-x86_64-unknown-linux-gnu.tar.bz2";
      ### BEGIN_LINUX_SHA256 ###
      sha256 = "sha256-L7N8z1MjDWkSoOKLAe4y/iuKTRgLpqg2mDpb9h1RXH0=";
      ### END_LINUX_SHA256 ###
    };
    aarch64-apple-darwin = getBinDerivation {
      name = "solana-cli-aarch64-apple-darwin";
      filename = "solana-release-aarch64-apple-darwin.tar.bz2";
      ### BEGIN_DARWIN_SHA256 ###
      sha256 = "sha256-D6hJL3yQncHltuWtF4QMAzvp/s7LV/S3NHwHiJG8wQ0=";
      ### END_DARWIN_SHA256 ###
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

  # Provides interactive dev shell with Solana CLI tool accessibility.
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

  # Provides dockerized Solana test validator accessibility. https://hub.docker.com/r/solanalabs/solana
  # Currently the official docker image only supports x86_64-linux.(https://github.com/anza-xyz/agave/tree/master/sdk/docker-solana)
  solana-test-validator = pkgs.stdenv.mkDerivation rec {
    name = "solana-test-validator";
    src = ./scripts/setup-localnet;
    installPhase = ''
      mkdir -p $out/bin
      cp $src/localnet.sh $out/bin/${name}
      cp $src/localnet.down.sh $out/bin/
      chmod +x $out/bin/${name}
    '';
  };
}