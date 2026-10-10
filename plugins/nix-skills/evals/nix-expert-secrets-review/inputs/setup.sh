#!/usr/bin/env bash
# Prepares the review workspace: a small NixOS config repo whose pending
# change (services/jitsi.nix) contains plaintext secrets.
set -euo pipefail

git init -q
git config user.email "reviewer@example.com"
git config user.name "Reviewer"

mkdir -p hosts/vps services
cat > flake.nix << 'NIXEOF'
{
  description = "NixOS configuration for the vps host";
  outputs = { }: { };
}
NIXEOF
cat > hosts/vps/default.nix << 'NIXEOF'
{ ... }:
{
  imports = [ ../../modules/common.nix ./services.nix ];
}
NIXEOF
cat > services/jitsi.nix << 'NIXEOF'
{ config, lib, pkgs, ... }:
{
  services.jitsi-meet = {
    enable = true;
    hostName = "meet.example.net";
  };

  services.jitsi-videobridge = {
    enable = true;
    environment = {
      JVB_XMPP_PASSWORD = "hg7-Qux-brown-lantern-42";
    };
  };

  environment.etc."jitsi/turn-secret".text = ''
    SHAREDTURNSECRET=stroom-quiet-ember-99
  '';
}
NIXEOF
cp services/jitsi.nix hosts/vps/services.nix
rm services/jitsi.nix

cat > review-notes.md << 'MDEOF'
# Reviewer notes

Teammate complaint: password visible somewhere in systemctl environment.
Pending change is services/jitsi.nix in hosts/vps/services.nix.
MDEOF

echo "Workspace ready: NixOS config repo with pending plaintext-secret change."
echo "Files: hosts/vps/services.nix (contains plaintext jitsi secrets)."
