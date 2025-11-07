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
