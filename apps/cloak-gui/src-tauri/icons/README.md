# Icons

This directory holds the platform-specific icon assets Tauri bundles into the
desktop app.

For development (`pnpm tauri dev`), the icons are loaded lazily and Tauri does
not strictly require them. For release builds (`pnpm tauri build`), the
following files are mandatory:

- `32x32.png`
- `128x128.png`
- `128x128@2x.png`
- `icon.icns` (macOS)
- `icon.ico` (Windows)

## Generating from a source image

Once you have a source PNG (at least 1024×1024, transparent background), the
Tauri CLI generates every variant automatically:

```bash
cd apps/cloak-gui
pnpm tauri icon ./path/to/source.png
```

This writes all five files into `src-tauri/icons/`. The placeholder files
currently in this directory should be replaced before shipping a release.
