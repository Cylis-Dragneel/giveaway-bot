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
          static = true;
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
            };

          };
          dockerfile = pkgs.writeText "Dockerfile" ''
            # Stage 1: Build with Nix (in a container)
            FROM nixos/nix:2.24 AS builder

            # Enable flakes
            ENV NIX_CONFIG="experimental-features = nix-command flakes"

            # Copy only the flake (lockfile included)
            WORKDIR /src
            COPY flake.nix flake.lock ./
            COPY . .

            # Build the static binary + Docker image tarball
            RUN nix build .#bot --print-out-paths > /tmp/bot-path && \
                nix build .#dockerImage --print-out-paths > /tmp/image-path

            # Extract the binary
            RUN cp $(cat /tmp/bot-path)/bin/giveaway-bot /giveaway-bot

            RUN mkdir /image && \
                tar -C /image -xf $(cat /tmp/image-path)

            # Stage 2: Runtime – copy from Nix-built layer
            FROM scratch

            # Copy the exact Nix-built layer
            COPY --from=builder /image /

            # Run
            CMD ["/bin/giveaway-bot"]
          '';

          default = bot;
        };
        devShells.default = pkgs.mkShellNoCC {
          packages = with pkgs; [
            go
            gotools
            golangci-lint
            gopls
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
