---
name: nix-expert
description: >
  Expert guidance on Nix, NixOS, home-manager, flakes, and Nix packaging. Use
  when writing or debugging Nix expressions, configuring NixOS or home-manager,
  authoring derivations, managing flake inputs, encrypting secrets with agenix
  or sops-nix, or hardening a NixOS host. Triggers on Nix, NixOS, nixpkgs,
  home-manager, flake.nix, nixos-rebuild, nix build, nix develop, devShell,
  derivation, overlay, agenix, and on Nix errors such as hash mismatches,
  infinite recursion, or evaluation failures. Advisory rather than generative:
  it explains, reviews, and debugs. If the repository provides its own skill for
  authoring host configurations, prefer that for writing config.
---

# Nix Expert

Advisory guidance on Nix, NixOS, and home-manager. This skill answers questions,
explains patterns, and reviews Nix code.

## Scope

Use this skill for:

- Explaining or reviewing Nix expressions and module definitions
- Authoring derivations and overlays
- Flake input management and lockfile hygiene
- Debugging build and evaluation failures
- home-manager configuration
- Secrets management patterns
- NixOS security hardening

When the task is to author or change `.nix` files for a specific host, check
whether the target repository ships its own config-generation skill and prefer
it — it will know that flake's layout, secrets backend, and conventions, which
this skill deliberately does not assume. Failing that, read the existing tree
before writing anything, and conform to what is already there.

## Routing

Read the matching reference file rather than answering from this file when the
request is specific. Each one is a task-oriented procedure, not passive
background:

| Topic | Trigger | Reference |
|-------|---------|-----------|
| **Build** | "build nix package", "nixos-rebuild build", "compile nix" | [references/Build.md](references/Build.md) |
| **Debug** | "debug nix", "nix error", "evaluation error", "infinite recursion" | [references/Debug.md](references/Debug.md) |
| **Develop** | "development shell", "nix develop", "devShell", "direnv" | [references/Develop.md](references/Develop.md) |
| **Deploy** | "deploy nixos", "nixos-rebuild switch", "remote deployment" | [references/Deploy.md](references/Deploy.md) |
| **Package** | "create package", "derivation", "buildGoModule", "package app" | [references/Package.md](references/Package.md) |
| **Flakes** | "create flake", "flake.lock", "update inputs", "flake outputs" | [references/Flakes.md](references/Flakes.md) |
| **Secrets** | "manage secrets", "agenix", "encrypt secrets", "age encryption" | [references/Secrets.md](references/Secrets.md) |
| **Security** | "harden nixos", "apparmor", "firewall", "security hardening" | [references/Security.md](references/Security.md) |
| **Troubleshoot** | "hash mismatch", "nix failing", "common errors" | [references/Troubleshoot.md](references/Troubleshoot.md) |

Read only the one you need. For general guidance, continue with this file.

## Core Principles

### 1. Declarative over imperative

```nix
# Good
services.nginx.enable = true;

# Bad — imperative escape hatch
systemd.services.nginx.postStart = "systemctl start nginx";
```

### 2. Reproducibility

Same inputs produce the same outputs. Pin versions explicitly, commit
`flake.lock`, and avoid impure operations (`builtins.getEnv`, `--impure`,
unpinned `fetchTarball`).

### 3. Modularity

```nix
# Good
imports = [
  ./hardware.nix
  ./networking.nix
  ./services.nix
];
```

### 4. Version control everything

Track all Nix configuration in git. Always put the `nix flake update` output in
the commit message body — a `flake.lock` diff is bare hashes and epochs, so the
generated summary is the only readable record of which inputs moved and where.
`nix flake update --commit-lock-file` does this for you. Add the *why* above it
when an input is pinned or overridden.

### 5. Prefer flakes

Flakes give hermetic evaluation, a standard output schema, dependency locking,
and better caching.

## NixOS Configuration Patterns

### Host configuration structure

```
hosts/<hostname>/
├── default.nix    # Imports modules, host-specific config
├── boot.nix       # Bootloader, initrd, kernel modules
└── hardware.nix   # Hardware settings, filesystems, mounts
```

Layouts vary between repositories. Read the existing tree before assuming one.

### Shared module organization

```
modules/
├── common/        # Baseline imported by every host
├── profiles/      # Opt-in role profiles (bare-metal, vm-guest, nvidia)
└── services/      # One file per service
```

### A mkHost helper

Many flakes wrap host construction to avoid repetition:

```nix
nixosConfigurations = {
  some-host = libx.mkHost {
    hostname = "some-host";
    system = "x86_64-linux";
    profile = "bare-metal";
  };
};
```

If the repository has such a helper, use it rather than calling
`nixpkgs.lib.nixosSystem` directly.

## Module Best Practices

### Define options properly

```nix
{ config, lib, pkgs, ... }:

{
  options.services.myservice = {
    enable = lib.mkEnableOption "my service";

    port = lib.mkOption {
      type = lib.types.port;
      default = 8080;
      description = "Port to listen on";
    };

    configFile = lib.mkOption {
      type = lib.types.path;
      description = "Path to configuration file";
    };
  };

  config = lib.mkIf config.services.myservice.enable {
    # Implementation
  };
}
```

### Choosing a type

The full set is documented in the NixOS manual under [Options
Types](https://nixos.org/manual/nixos/stable/#sec-option-types) and defined in
`lib/types.nix` in nixpkgs. Read one of those rather than working from memory —
the set grows, and composed types (`either`, `oneOf`, `coercedTo`, `attrTag`)
are easy to misremember.

Pick the narrowest type that fits: `types.port` rather than `types.int`,
`types.enum` rather than `types.str` when the value set is closed. The module
system then rejects bad values at eval time instead of at activation.

### mkIf, mkMerge, mkDefault

```nix
config = lib.mkIf config.services.myservice.enable { };

config = lib.mkMerge [
  { always.present = true; }
  (lib.mkIf condition { conditional.value = true; })
];

# Overridable default
services.myservice.port = lib.mkDefault 8080;
```

`mkDefault` has priority 1000, `mkForce` has 50, a plain assignment has 100.
Use `mkForce` sparingly — it defeats the merge system.

## Package Development

Derivation templates, the `callPackage` pattern, and per-language builders
(`buildGoModule`, `rustPlatform`, `python3Packages`) live in
[references/Package.md](references/Package.md). Read that rather than working from
memory — `meta` attributes and hash arguments are easy to get subtly wrong.

### Overlays

```nix
{ inputs }:
{
  additions = final: _prev: import ../pkgs { pkgs = final; };

  modifications = final: prev: {
    somepackage = prev.somepackage.overrideAttrs (old: {
      version = "custom";
    });
  };
}
```

Use `final` for anything that should see later overlays, `prev` for the input
you are modifying. Referencing `final.foo` inside a definition of `foo` causes
infinite recursion.

## Flake Management

```bash
nix flake lock              # Lock dependencies
nix flake update            # Update all inputs
nix flake update nixpkgs    # Update one input
nix flake check             # Validate outputs
nix flake show              # List outputs
nix flake metadata          # Show inputs and revision
```

```nix
{
  description = "Flake description";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }: {
    nixosConfigurations = { };
    homeConfigurations = { };
    packages = { };
    devShells = { };
  };
}
```

## Home-Manager Patterns

### Session variables

```nix
home.sessionVariables = {
  EDITOR = "vim";
  BROWSER = "firefox";
};
```

### XDG config files

```nix
# Symlink a static file
xdg.configFile."myapp/config.yml".source = ./myapp-config.yml;

# Generate dynamically
xdg.configFile."myapp/generated.conf".text = ''
  setting1 = ${someValue}
'';

# Executable
xdg.configFile."bin/script.sh" = {
  source = ./script.sh;
  executable = true;
};
```

### User services

```nix
systemd.user.services.myservice = {
  Unit = {
    Description = "My Service";
    After = [ "network.target" ];
  };
  Service = {
    ExecStart = "${pkgs.mypackage}/bin/myservice";
    Restart = "on-failure";
  };
  Install.WantedBy = [ "default.target" ];
};
```

## Secrets Management

Never put plaintext secrets in a `.nix` file — everything in the Nix store is
world-readable. Use `agenix` or `sops-nix`, and reference the decrypted path at
runtime.

```nix
# secrets.nix
let
  user   = "ssh-ed25519 AAAAC3...";
  system = "ssh-ed25519 AAAAC3...";
in {
  "secret.age".publicKeys = [ user system ];
}
```

```nix
{
  age.secrets.mySecret = {
    file = ../secrets/mySecret.age;
    owner = "myuser";
    group = "mygroup";
  };

  # Pass the path, never the value
  services.myservice.passwordFile = config.age.secrets.mySecret.path;
}
```

```bash
agenix -e secrets/mySecret.age   # Edit or create
agenix -r                        # Re-key after adding a host
```

See [references/Secrets.md](references/Secrets.md) for the full treatment.

## Safety and Testing

`switch` is the only irreversible-in-the-moment step. Climb the ladder in
order and stop at the first failure — never skip a rung to save time.

| # | Command | Proves | On failure |
|---|---------|--------|------------|
| 1 | `nixos-rebuild dry-build --flake .#<host>` | Evaluates and shows what *would* be built | Fix eval errors; re-run 1 |
| 2 | `nixos-rebuild build --flake .#<host>` | It actually compiles | Read the failing derivation; re-run 2 |
| 3 | `nixos-rebuild dry-activate --flake .#<host>` | Which units would restart or stop | Reconsider blast radius; back to 1 |
| 4 | `nixos-rebuild test --flake .#<host>` | Activates now, bootloader untouched | `reboot` recovers the old generation |
| 5 | `nixos-rebuild switch --flake .#<host>` | Activates and makes it the boot default | `nixos-rebuild switch --rollback` |

Rung 3 is the one people skip, and the one that catches "this restarts
postgresql and sshd" before it happens.

Confirm each rung actually succeeded before advancing — check the exit status,
don't assume. If rung 2 fails, rungs 3-5 are meaningless.

### Extra care on remote hosts

Never `switch` a remote host you cannot physically reach without first running
rungs 1-4. A bad `networking` or `openssh` change locks you out, and rung 5
makes it survive the reboot that would otherwise have saved you.

```bash
# Build locally, push the closure, activate with a safety net
nixos-rebuild test --flake .#<host> --target-host root@<host> --build-host localhost
```

Consider `services.openssh.enable` and firewall changes one-at-a-time, and keep
a second SSH session open while testing.

### Rollback

```bash
nixos-rebuild list-generations              # See what you can return to
nixos-rebuild switch --rollback             # Previous generation
nixos-rebuild switch --switch-generation N  # A specific one
```

Keep at least two or three recent generations. `nix-collect-garbage -d` deletes
every rollback target — never run it as a first response to a full disk, and
never immediately after a `switch` you have not yet verified.

See [references/Deploy.md](references/Deploy.md) for remote deployment, and
[references/Troubleshoot.md](references/Troubleshoot.md) when a rung fails.

## Common Patterns

### Conditional imports

```nix
imports = [
  ./base.nix
] ++ lib.optionals (desktop != null) [
  ./desktop/${desktop}
];
```

### String interpolation

```nix
message = "Hello ${name}";

config = ''
  setting1 = ${value1}
'';

# Escaping $ inside an indented string
script = ''
  echo "Nix variable: ${nixVar}"
  echo "Shell variable: ''${shellVar}"
'';
```

### List and attrset operations

```nix
all      = list1 ++ list2;
filtered = lib.filter (x: x > 5) list;
doubled  = map (x: x * 2) list;

merged   = set1 // set2;                       # Shallow
merged   = lib.recursiveUpdate set1 set2;      # Deep
filtered = lib.filterAttrs (n: v: v != null) attrs;
mapped   = lib.mapAttrs (n: v: v * 2) attrs;
```

## Debugging

```nix
value = lib.traceVal someExpression;
value = lib.traceSeq "message" someExpression;
```

```bash
nix eval .#nixosConfigurations.<host>.config.services.nginx.enable
nix derivation show .#package
nix path-info -Sh .#package
nix why-depends .#package .#dependency
```

Add `--show-trace` to any failing evaluation for the full stack.

### Hash mismatch

```bash
nix-prefetch-github owner repo --rev <commit>
```

For Go modules, set `vendorHash = lib.fakeHash;` and read the correct hash out
of the build failure.

### Infinite recursion

Usually a `config` value read at the top level of a module, or an overlay
referring to `final.foo` while defining `foo`. Move the read inside `config` or
switch to `prev`.

## Performance

- Use binary caches; add project caches via `nixConfig.extra-substituters`
- Keep `flake.lock` updated but stable — churn invalidates caches
- Use `nix-direnv` so dev shells are cached and GC-rooted
- Avoid expensive list operations inside module option defaults

## Security

### Quick wins

```nix
{
  networking.firewall.enable = true;

  services.openssh.settings = {
    PermitRootLogin = "no";
    PasswordAuthentication = false;
  };

  system.autoUpgrade = {
    enable = true;
    allowReboot = false;
  };

  security.apparmor.enable = true;
}
```

### Checklist

- [ ] Firewall enabled, default deny
- [ ] SSH hardened — no root login, no password auth
- [ ] Secrets encrypted, referenced by path not value
- [ ] Automatic updates configured
- [ ] AppArmor or per-service systemd hardening enabled
- [ ] Audit logging on critical services
- [ ] Minimal package set on exposed hosts

See [references/Security.md](references/Security.md) for the hardened profile and per-service
sandboxing.

## Resources

- NixOS Manual: <https://nixos.org/manual/nixos/stable/>
- Nixpkgs Manual: <https://nixos.org/manual/nixpkgs/stable/>
- Home-Manager Manual: <https://nix-community.github.io/home-manager/>
- Nix Pills: <https://nixos.org/guides/nix-pills/>
- Option search: <https://search.nixos.org/options>
