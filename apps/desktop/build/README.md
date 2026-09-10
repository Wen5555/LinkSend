# Build Directory

The build directory houses the Wails 3 (`v3.0.0-beta.18`) build files and assets for LinkSend.

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails3 task darwin:package`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and resource files used by `wails3 task build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails3 task build`.

- `icon.ico` - The icon used for the application. This is used when building using `wails3 task build`. If you wish to
  use a different icon, simply replace this file with your own. If it is missing, a new `icon.ico` file
  will be created using the `appicon.png` file in the build directory.
- `installer/*` - Historical installer assets; the active Wails 3 Taskfile uses the generated `windows/nsis` path.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.
