{
  description = "Multi-agent orchestration system for Claude Code with persistent work tracking";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    beads.url = "github:mineshafthall/beads";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      beads,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
        beadsPkg = beads.packages.${system}.default;
      in
      {
        packages = {
          ms = pkgs.buildGoModule {
            pname = "ms";
            version = "1.0.0";
            src = ./.;
            vendorHash = "sha256-mJzpsl4XnIm3ZSg7fFn0MOdQQW1bdOkAJ+TikiLMXJM=";

            ldflags = [
              "-X github.com/mineshafthall/mineshaft/internal/cmd.Build=nix"
              "-X github.com/steveyegge/mineshaft/internal/cmd.BuiltProperly=1"
            ];

            subPackages = [ "cmd/ms" ];

            meta = with pkgs.lib; {
              description = "Multi-agent orchestration system for Claude Code with persistent work tracking";
              homepage = "https://github.com/mineshafthall/mineshaft";
              license = licenses.mit;
              mainProgram = "ms";
            };
          };
          default = self.packages.${system}.ms;
        };

        apps = {
          ms = flake-utils.lib.mkApp {
            drv = self.packages.${system}.ms;
          };
          default = self.apps.${system}.ms;
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            beadsPkg
            pkgs.go_1_25
            pkgs.gopls
            pkgs.gotools
            pkgs.go-tools
          ];
        };
      }
    );
}
