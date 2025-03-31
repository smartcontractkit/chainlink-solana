{ system ? builtins.currentSystem
, pkgs ? import <nixpkgs> { inherit system; }
,
}:

pkgs.buildGo123Module {
  name = "solutil";
  src = ./.;
  vendorHash = "sha256-LsDgarKEv1ge0y/mk+TUtGSCGKCVX0L7gBY3mRR8KLk=";
}

