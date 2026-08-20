# Release Artifacts

Release builds ship for **two platforms**: Windows and macOS. The desktop client
is packaged with Tauri, and `bundle.targets` is `"all"`, so each platform's build
host produces that platform's native installers.

## What each platform produces

| Platform | Build host | Artifacts |
| --- | --- | --- |
| Windows | `windows-latest` | `.msi` (WiX) and `.exe` (NSIS, per-machine install) |
| macOS | `macos-latest` | `.app` bundle and `.dmg` disk image |

## Where they come from

`.github/workflows/release.yml` runs a two-OS matrix on a `v*` tag (or manual
dispatch). Each job builds the frontend, compiles the Tauri client, and uploads
its installers as workflow artifacts (`Windows-installers`, `macOS-installers`).

Tauri writes bundles to `desktop/src-tauri/target/release/bundle/` on the build
host:

```
bundle/
  dmg/    *.dmg     (macOS)
  macos/  *.app     (macOS)
  msi/    *.msi     (Windows)
  nsis/   *.exe     (Windows)
```

## This folder

Downloaded or locally built installers can be staged here per version, e.g.
`release/0.1.0/`. Binaries are git-ignored (see the root `.gitignore`); only this
README and the version folder structure are tracked.

## Local builds

```bash
# from repo root, on the target OS
cd desktop/src-tauri
cargo tauri build        # or: npx @tauri-apps/cli build
```

The `beforeBuildCommand` in `tauri.conf.json` builds the frontend first, so a
plain `tauri build` produces a complete installer.

## Notes

- macOS `.dmg` bundling has failed in earlier local runs; the `.app` bundle
  builds successfully. Track `.dmg` packaging separately.
- Signing and notarization (macOS) and code signing (Windows) are not yet
  configured; add signing identities before public distribution.
