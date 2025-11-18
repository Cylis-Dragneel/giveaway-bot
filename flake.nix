{
  description = "Dev shell + static binary + Docker image";

  inputs = {
    nixpkgs.url = "https://flakehub.com/f/NixOS/nixpkgs/0.1.*"; # unstable
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { self, flake-utils, ... }@inputs:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        # Go version (change once to bump everything)
        goVersion = 25;
        overlay = final: prev: {
          go = final."go_1_${toString goVersion}";
        };
        pkgs = import inputs.nixpkgs {
          inherit system;
          overlays = [ overlay ];
        };

        bot = pkgs.buildGo124Module {
          pname = "giveaway-bot";
          version = "0.1.0";

          src = ./.; # whole repo (exclude .git, flake.lock, etc.)
          vendorHash = "sha256-5F+MQJjFp/nYSAreHLKYMcWQPShVXnIThagzRIleWPg=";

          ldflags = [
            "-s"
            "-w"
            "-X main.version=${self.shortRev or "dev"}"
          ];

          # embed git commit
          preBuild = ''
            export GIT_COMMIT=${self.rev or "dirty"}
          '';
        };
      in
      {
        overlays.default = overlay;
        packages = {
          inherit bot;
          dockerImage = pkgs.dockerTools.buildLayeredImage {
            name = "giveaway-bot";
            tag = "latest";
            contents = [
              bot
            ];
            config = {
              Cmd = [ "${bot}/bin/giveaway-bot" ];
              WorkingDir = "/app";
            };
          };
          default = bot;
        };
        devShells.default = pkgs.mkShellNoCC {
          packages = with pkgs; [
            go
            gotools
            golangci-lint
            gopls
            gofumpt
            gcc
          ];
        };
        apps.default = {
          type = "app";
          program = "${bot}/bin/giveaway-bot";
        };
      }
    );
}
