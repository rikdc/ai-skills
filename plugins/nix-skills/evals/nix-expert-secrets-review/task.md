# Review a Jitsi module before it merges

A friend's NixOS config repo has a pending change adding a self-hosted Jitsi
Meet module. Please review it before it merges. The module:

```nix
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
```

A teammate says the password shows up somewhere it shouldn't, and suggested an
easy fix: move the two secrets into a `secrets.env` file, remove it from git,
`chmod 600` it, and reference the file path.

Run `bash setup.sh` first to prepare the repo (the module is at
`hosts/vps/services.nix`). Write your review to `review.md`. This is an offline
review; don't run network commands.
