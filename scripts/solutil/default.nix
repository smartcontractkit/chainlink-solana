{ system ? builtins.currentSystem
, pkgs ? import <nixpkgs> { inherit system; }
,
}:

pkgs.buildGo123Module {
  name = "solutil";
  src = ./.;
  vendorHash = "sha256-onIj3yhEc8UOeKyFGXkVCo7QLsvXb8tmgjEECmbg9iU=";
}

