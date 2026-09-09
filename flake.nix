{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    git-hooks.url = "github:cachix/git-hooks.nix";
    git-hooks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs = {
    self,
    nixpkgs,
    git-hooks,
  }: let
    systems = ["x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin"];
    forAllSystems = f:
      nixpkgs.lib.genAttrs systems (system: f system nixpkgs.legacyPackages.${system});
  in {
    checks = forAllSystems (system: pkgs: {
      pre-commit = git-hooks.lib.${system}.run {
        src = ./.;
        hooks = {
          alejandra.enable = true;
          # golangci-lint and gotest shell out to `go`, which the hook env lacks
          golangci-lint = {
            enable = true;
            extraPackages = [pkgs.go];
          };

          gotest = {
            enable = true;
            stages = ["pre-push"];
            extraPackages = [pkgs.go];
          };

          nix-build = {
            enable = true;
            entry = pkgs.lib.getExe (pkgs.writeShellApplication {
              name = "nix-build-check";
              runtimeInputs = [pkgs.nix];
              text = "nix build --no-link";
            });
            stages = ["pre-push"];
            pass_filenames = false;
            files = "go\\.(mod|sum)|flake\\.nix";
          };
        };
      };
    });

    packages = forAllSystems (system: pkgs: {
      default = pkgs.buildGoModule {
        pname = "nagomi-core";
        version = self.shortRev or self.dirtyShortRev or "dev";
        src = ./.;
        vendorHash = "sha256-R4AHEa2t2cclh0QTOLYWYn+sm2DZm4WR6+aU2NdynX8=";
        subPackages = ["cmd/nagomi"];
      };
    });

    devShells = forAllSystems (system: pkgs: {
      default = pkgs.mkShell {
        packages = with pkgs; [
          go
          golangci-lint
          air

          buf
          protoc-gen-go
          protoc-gen-go-grpc
          protoc-gen-connect-go

          sqlc
          goose
          grpcurl

          (writeShellScriptBin "run" ''
            exec ${air}/bin/air -build.cmd "go build -o ./tmp/main ./cmd/nagomi/main.go" -build.bin ./tmp/main
          '')

          (writeShellScriptBin "regen" ''
            rm -rf internal/db/sqlc internal/gen
            ${sqlc}/bin/sqlc generate
            ${buf}/bin/buf generate
          '')

          (writeShellScriptBin "migrate" ''
            exec ${goose}/bin/goose -dir internal/db/migrations postgres "$DATABASE_URL" up
          '')

          (writeShellScriptBin "cover" ''
            go test -coverprofile=coverage.out ./... &&
              go tool cover -html=coverage.out -o coverage.html
          '')

          (writeShellScriptBin "test-db" ''
            TEST_DATABASE_URL="''${TEST_DATABASE_URL:-''${DATABASE_URL:?set TEST_DATABASE_URL or DATABASE_URL}}" \
              exec go test ./internal/db/... -v
          '')

          (writeShellScriptBin "bump-protos" ''
            set -e
            git submodule update --remote --checkout proto
            git add proto
            git commit -m "chore: bump protos"
            git push
          '')
        ];

        shellHook = self.checks.${system}.pre-commit.shellHook;
      };
    });

    formatter = forAllSystems (system: pkgs: pkgs.alejandra);
  };
}
