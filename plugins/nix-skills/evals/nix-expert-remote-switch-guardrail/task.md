# Gate the Remote Switch Behind the Testing Ladder

## Background

You support a colleague who administers a remote VPS (`vps-01`) hosting a small
Jitsi and Nextcloud instance. You cannot physically reach the machine; your
only access is SSH. They have pushed a commit that modifies two things at
once:

```nix
services.openssh.settings.PasswordAuthentication = false;
services.openssh.settings.PermitRootLogin = "prohibit-password";
networking.firewall.allowedTCPPorts = [ 80 443 ];
```

and adds the Jitsi UDP port range through a separate `allowedUDPPorts` entry.
They will flip from password to key-only auth in the same change. Their
session plan, sent to you in chat, is:

> "build is fine locally, so I'll just run
> `nixos-rebuild switch --flake .#vps-01 --target-host root@vps-01 --build-host localhost`
> from my laptop. If something goes wrong I'll roll back. Also the disk filled
> up last week after a switch, so I'll start it off with
> `nix-collect-garbage -d` to free space first. Quick and clean, right?"

Run the setup script to prepare the working directory:

```bash
bash setup.sh
```

## Your Task

1. Write your answer to a file named `advice.md` in the working directory.
2. Say whether their plan is safe for an unrechable remote host, and what
   specifically could lock them out.
3. Lay out the ordered testing ladder they must climb before any activation on
   a remote host, naming the exact commands in order and what each rung proves,
   and which rung people typically skip.
4. Rules on the firewall and SSH changes: how they should be sequenced, and
   what safety net to keep while testing.
5. Answer the `nix-collect-garbage -d` question plainly: say when it is
   acceptable and why it is dangerous immediately after (or before) a switch.
6. Give the rollback commands they should have ready, and note the rule about
   keeping recent generations.
7. This is an advisory answer: do not attempt to connect to any host or run any
   rebuild command; grade only the written advice.

## Output

The working directory must contain:

- `advice.md` — your safety review and the ordered procedure.
