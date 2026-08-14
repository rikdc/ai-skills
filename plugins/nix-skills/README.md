# Nix Skills Plugin

Expert guidance on Nix, NixOS, and home-manager.

## Skills Included

### `/nix-expert`

Advisory guidance on Nix expressions, modules, derivations, overlays, flake
management, home-manager, secrets, and NixOS hardening.

**Use when**: writing or reviewing Nix expressions, authoring packages,
managing flake inputs, debugging evaluation and build failures, or hardening a
host.

Routes to nine focused reference files, each a task-oriented procedure rather
than passive background:

| Reference | Covers |
|---|---|
| [Build](skills/nix-expert/references/Build.md) | `nix build`, `nixos-rebuild build`, build failures |
| [Debug](skills/nix-expert/references/Debug.md) | Evaluation errors, infinite recursion, `--show-trace` |
| [Develop](skills/nix-expert/references/Develop.md) | `devShell`, `nix develop`, direnv integration |
| [Deploy](skills/nix-expert/references/Deploy.md) | `nixos-rebuild switch`, remote deployment, rollback |
| [Package](skills/nix-expert/references/Package.md) | Derivations, `buildGoModule`, overlays |
| [Flakes](skills/nix-expert/references/Flakes.md) | Inputs, `flake.lock`, outputs schema |
| [Secrets](skills/nix-expert/references/Secrets.md) | agenix, sops-nix, age encryption |
| [Security](skills/nix-expert/references/Security.md) | Hardening, AppArmor, firewall, systemd sandboxing |
| [Troubleshoot](skills/nix-expert/references/Troubleshoot.md) | Hash mismatches, common errors |

## The rebuild safety ladder

`SKILL.md` carries an explicit five-rung ladder for `nixos-rebuild`, since
`switch` is the one step that is not trivially reversible:

```
dry-build → build → dry-activate → test → switch
```

Each rung records what it proves and how to recover when it fails. `dry-activate`
is the rung people skip, and the one that reveals which units a `switch` would
restart before it happens. There is additional guidance for remote hosts, where
a bad `networking` or `openssh` change survives the reboot that would otherwise
have recovered it.

## Scope

This skill is advisory. It answers questions and reviews Nix code; it does not
author host configurations for a specific fleet. Repository-specific config
generation is a poor fit for a published skill — it depends on one flake's
layout, secrets backend, reverse proxy, and storage model, and the specificity
that makes such a tool useful is exactly what makes it unpublishable. Keep that
kind of skill local to the repository it serves.

## Installation

Install via the Claude Code marketplace:

```bash
claude code plugins install nix-skills
```

Or install from this repository:

```bash
claude code plugins install github:rikdc/ai-skills/nix-skills
```

## Usage Examples

```bash
# Debug an evaluation failure
/nix-expert Why does this module cause infinite recursion?

# Package an application
/nix-expert Package this Go application for Nix

# Understand a concept
/nix-expert How do overlays work, and when should I use final vs prev?

# Deploy safely
/nix-expert What is the safe order for rebuilding a remote host?

# Harden a host
/nix-expert How should I harden an internet-facing NixOS host?

# Secrets
/nix-expert Should I use agenix or sops-nix for this?
```

## Notes on Accuracy

Nix option names and shapes drift between releases. The reference files favour
current stable patterns, but always confirm an option against the nixpkgs
revision your repository actually pins — see
<https://search.nixos.org/options>. Where guidance here and the release
disagree, the release is right.

## License

Mozilla Public License 2.0 — see [LICENSE](../../LICENSE).
