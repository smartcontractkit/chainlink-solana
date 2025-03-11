{ system ? builtins.currentSystem
, pkgs ? import <nixpkgs> { inherit system; }
,
}:

pkgs.buildGoModule {
  name = "solutil";
  src = ./.;
  vendorHash = "sha256-se09IJLkE/qCs+VV0wE/9dooZnhtJrF97ABqmwk158c=";
}

