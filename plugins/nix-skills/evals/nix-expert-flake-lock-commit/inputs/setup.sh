#!/usr/bin/env bash
# Prepares a git repo with a staged flake.lock change and a saved dry-run
# update summary, with no commit yet.
set -euo pipefail

git init -q
# Keep fixture files out of the repository's status, whichever layout the
# harness uses (inputs/ preserved, or flattened into the working directory).
printf 'inputs/\nsetup.sh\n' >> .git/info/exclude
git config user.email "nix-admin@example.com"
git config user.name "Nix Admin"
git config commit.gpgsign false
# The machine's global gitignore ignores flake.lock; this scenario needs it tracked.
git config core.excludesfile /dev/null
git checkout -q -b main

cat > flake.nix << 'NIXEOF'
{
  description = "NixOS configuration for my hosts";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-25.05";
    home-manager = {
      url = "github:nix-community/home-manager?rev=3a1f2cbb4b55d4c35a3ea56f4a2a2b57d9e21a44";
      # Pinned to release-25.05 pending fix for the waybar regression
    };
  };

  outputs = { self, nixpkgs, home-manager }: {
    nixosConfigurations = { };
  };
}
NIXEOF

cat > flake.lock << 'LOCKEOF'
{
  "nodes": {
    "home-manager": {
      "locked": {
        "lastModified": 1750000000,
        "rev": "3a1f2cbb4b55d4c35a3ea56f4a2a2b57d9e21a44",
        "type": "github",
        "owner": "nix-community",
        "repo": "home-manager"
      },
      "original": {
        "type": "github",
        "owner": "nix-community",
        "repo": "home-manager",
        "rev": "3a1f2cbb4b55d4c35a3ea56f4a2a2b57d9e21a44"
      }
    },
    "nixpkgs": {
      "locked": {
        "lastModified": 1749900000,
        "rev": "b9c9b0af9f4c72b4d51db3e1a4f6f2c0b8a1d3e5",
        "type": "github",
        "owner": "nixos",
        "repo": "nixpkgs"
      }
    },
    "root": {}
  },
  "version": 7
}
LOCKEOF

mkdir -p hosts notes
cat > hosts/vps.nix << 'NIXEOF'
{ ... }:
{
  fileSystems."/" = { device = "/dev/disk/by-label/nixos"; fsType = "ext4"; };
}
NIXEOF
git add flake.nix flake.lock hosts/vps.nix
git commit -q -m "chore: initial flake and vps host"

# The (offline, prepared) lock update: staged, uncommitted.
cat > flake.lock << 'LOCKEOF'
{
  "nodes": {
    "home-manager": {
      "locked": {
        "lastModified": 1750200000,
        "rev": "7c2a4e8f9b0a1d2c3e4f5a6b7c8d9e0f1a2b3c4d",
        "type": "github",
        "owner": "nix-community",
        "repo": "home-manager"
      },
      "original": {
        "type": "github",
        "owner": "nix-community",
        "repo": "home-manager",
        "rev": "3a1f2cbb4b55d4c35a3ea56f4a2a2b57d9e21a44"
      }
    },
    "nixpkgs": {
      "locked": {
        "lastModified": 1750100000,
        "rev": "d4a2b7c9e1f3a5b7c9d1e3f5a7b9c1d3e5f7a9b1",
        "type": "github",
        "owner": "nixos",
        "repo": "nixpkgs"
      }
    },
    "root": {}
  },
  "version": 7
}
LOCKEOF
git add flake.lock

cat > notes/update-summary.txt << 'SUMEOF'
$ nix flake update home-manager
    Updated 'home-manager' in flake.lock:
        'github:nix-community/home-manager/3a1f2cbb4b55d4c35a3ea56f4a2a2b57d9e21a44' (2025-06-15) ->
        'github:nix-community/home-manager/7c2a4e8f9b0a1d2c3e4f5a6b7c8d9e0f1a2b3c4d' (2025-06-18)
    Updated 'nixpkgs' in flake.lock:
        'github:nixos/nixpkgs/b9c9b0af9f4c72b4d51db3e1a4f6f2c0b8a1d3e5' (2025-06-14) ->
        'github:nixos/nixpkgs/d4a2b7c9e1f3a5b7c9d1e3f5a7b9c1d3e5f7a9b1' (2025-06-16)
SUMEOF

echo "Repo ready on main with a staged flake.lock change (2 inputs moved) and no commit."
echo "Offline dry-run summary saved at notes/update-summary.txt."
