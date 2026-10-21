# Colleague's plan for vps-01

A colleague administers a remote VPS (`vps-01`, SSH access only) and sent me
this in chat:

> I've pushed a commit that sets `services.openssh.settings.PasswordAuthentication = false;`
> and `services.openssh.settings.PermitRootLogin = "prohibit-password";`, opens
> `networking.firewall.allowedTCPPorts = [ 80 443 ];`, and adds the Jitsi UDP
> range via `allowedUDPPorts`. It's all in the same change.
>
> build is fine locally, so I'll just run
> `nixos-rebuild switch --flake .#vps-01 --target-host root@vps-01 --build-host localhost`
> from my laptop. If something goes wrong I'll roll back. Also the disk filled
> up last week after a switch, so I'll start it off with
> `nix-collect-garbage -d` to free space first. Quick and clean, right?

Is this plan okay? Write your answer to `advice.md`.

Run `bash setup.sh` first; it puts the same change and plan in `notes/`. This
is advisory only: don't connect to any host or run any rebuild.
