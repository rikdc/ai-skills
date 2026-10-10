# Review the Jitsi Secrets Module

## Background

You are reviewing a pending change on a friend's NixOS configuration repository.
The tweak is a new module for a Jitsi Meet instance they self-host. The change
is sitting in `services/jitsi.nix` and is about to be merged, but a teammate
complained that "the password shows up in `localectl status` output
somewhere" and asked you to look before it lands. The current module looks like
this:

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

A teammate suggests "an easy fix": move the two secrets into a
`secrets.env` file, remove it from git and `chmod 600` it, then reference the
file path. Run the setup script to prepare the working directory:

```bash
bash setup.sh
```

## Your Task

1. Write your answer to a file named `review.md` in the working directory.
2. State plainly whether the change as written is acceptable, and say exactly
   where the plaintext secrets end up.
3. Recommend the secret-management pattern the Nix ecosystem actually supports
   for this case, and show the shape of the corrected module (how the secret is
   stored and how the service consumes it).
4. Evaluate the teammate's "easy fix" — say whether leaving a plaintext
   `secrets.env` on disk (even untracked and `chmod 600`) solves the problem,
   and what still goes wrong.
5. Do not run any network commands; this is an offline review, not a build.

## Output

The working directory must contain:

- `review.md` — your review and recommendation.
