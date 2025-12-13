{
  description = "Nostr egg sales bot";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go_1_23
            gotools
            gopls
            go-tools
            golangci-lint
            air
          ];

          shellHook = ''
            export PROJECT_NAME="eggbot"
            export PROJECT_MODULE="github.com/buildtall-systems/eggbot"
            echo "🚀 eggbot development environment"
            echo "   Go $(go version | awk '{print $3}')"
            echo ""
            echo "Quick commands:"
            echo "  make dev        - Run with live reload"
            echo "  make build      - Build binary"
            echo "  make test       - Run tests"
            echo "  make lint       - Run linter"
          '';
        };

        packages.default = pkgs.buildGoModule {
          pname = "eggbot";
          version = "0.1.0";
          src = ./.;
          vendorHash = null;
        };
      }
    );
}
