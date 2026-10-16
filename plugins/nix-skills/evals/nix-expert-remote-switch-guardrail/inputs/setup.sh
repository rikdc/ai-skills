#!/usr/bin/env bash
# Prepares the advisory workspace: notes describing the pending remote change
# and the colleague's switch plan.
set -euo pipefail

mkdir -p notes
cat > notes/pending-change.txt << 'EOF'
Pending change on vps-01 (remote VPS, SSH access only, no console):
  services.openssh.settings.PasswordAuthentication = false;
  services.openssh.settings.PermitRootLogin = "prohibit-password";
  networking.firewall.allowedTCPPorts = [ 80 443 ];
  networking.firewall.allowedUDPPorts = [ 10000-10100 ];
Also flipping the host from password login to key-only auth in the same change.
EOF

cat > notes/colleague-plan.txt << 'EOF'
Plan (from chat):
  "build is fine locally, so I'll just run
   nixos-rebuild switch --flake .#vps-01 --target-host root@vps-01 --build-host localhost
   from my laptop. If something goes wrong I'll roll back. Also the disk filled
   up last week after a switch, so I'll start it off with
   nix-collect-garbage -d to free space first. Quick and clean, right?"
EOF

echo "Workspace ready: advisory notes in notes/ (no git repo needed)."
echo "Files: notes/pending-change.txt, notes/colleague-plan.txt."
