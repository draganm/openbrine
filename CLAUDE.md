## Environment

This repo is a Nix flake with a devshell (see `flake.nix` and `.envrc`). All build/run/test commands must execute inside the devshell so the pinned toolchain is used.

- With direnv active, commands run from the project directory pick up the environment automatically.
- Otherwise, wrap commands as `nix develop -c <command>`.
- Do not rely on system-installed toolchains — defer to the flake.
