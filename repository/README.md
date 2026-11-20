# dareluna patcher 🎀

A pink-themed, offline Windows patch installer for a **Marvel Rivals** mod. The user selects a folder containing three verified original mod files and an output folder, then clicks **INSTALL PATCH**. The program creates the modified files without changing the originals.

> **Unofficial fan project.** Not affiliated with Marvel, NetEase, or the original mod creator. This repository is provided for source review. Distribution of a patch is subject to the original mod's permissions and relevant platform rules.

## What this repository contains

- `gui_windows.go` — Windows GUI, folder pickers, buttons, progress/status display.
- `core.go` — patch loading, SHA-256 verification, AES-GCM decryption, delta application and safe output installation.
- `payload/manifest.json` — expected source/output filenames, sizes and SHA-256 hashes.
- `payload/*.rvp` — encrypted, source-dependent binary delta data, **not** standalone original or output mod files.
- `build_payload.py` — utility for regenerating the patch payload from two ZIP files (requires those files separately).
- `build_windows.bat` — local Windows build script.
- `.github/workflows/build.yml` — automated Windows build from the checked-in repository.
- `Pink_Ribbon_Heart.png` and `.ico` — original UI/icon assets.
- `main_linux.go` — command-line test entry point for Linux; excluded from Windows builds by its build tag.

The original `.pak`, `.ucas`, `.utoc` files and finished modified game files are **not** included in this repository. The patch payload may contain third-party-derived data; assess redistribution permissions before making a repository public.

## Build the Windows executable

Requirements: [Go](https://go.dev/dl/) **1.22+**, Windows 64-bit. No third-party Go packages, Python, external delta tools or admin rights are required for the normal EXE build or installation.

1. Download or clone this repository, keeping `payload/` alongside the source files.
2. Open a terminal in the repository root.
3. Run `build_windows.bat`, or run the following commands in Command Prompt:

```bat
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w -H windowsgui" -o dareluna_patcher.exe .
```

The resulting file is `dareluna_patcher.exe`.

On Linux/macOS with Go installed, you can cross-compile with:

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -H windowsgui' -o dareluna_patcher.exe .
```

**GitHub Actions:** Open the **Actions** tab → **Build Windows EXE** → **Run workflow**. The workflow also runs on push. Download the `dareluna-patcher-windows` artifact after the run finishes.

## How to use the patcher

1. Obtain the **authorized original mod** and extract its ZIP to a folder containing exactly these filenames:

   - `Luna DD PJS DP_9999999_P.pak`
   - `Luna DD PJS DP_9999999_P.ucas`
   - `Luna DD PJS DP_9999999_P.utoc`

2. Launch `dareluna_patcher.exe` on Windows 10/11 (64-bit).
3. Use the first **Browse** control to select that original-mod folder.
4. Use the second **Browse** control to select an output folder (preferably empty).
5. Click **INSTALL PATCH**.

The patcher checks exact source SHA-256 hashes and creates:

- `Luna_DD_PJS_9999999_P.pak`
- `Luna_DD_PJS_9999999_P.ucas`
- `Luna_DD_PJS_9999999_P.utoc`

It verifies generated output hashes and will not overwrite conflicting files. It does **not** download mods, contact servers, modify game memory, bypass anti-cheat or run external commands. It is not guaranteed to be compatible with every game version.

## How the patch is generated / independently inspected

The patch creator script is included for full review. To regenerate the payload from **your own authorized source and target archives**, install Python 3.10+ and the `cryptography` package:

```bash
python -m pip install cryptography
python build_payload.py ORIGINAL.zip MODIFIED.zip --out payload
```

Each ZIP must contain exactly one `.pak`, `.ucas` and `.utoc` file at its root. `build_payload.py` creates copy/literal deltas, compresses them with zlib, encrypts them using a key derived from the corresponding original file content, and emits a manifest with SHA-256 checksums. `core.go` performs the reverse operation locally and validates the results. The algorithm and decryption logic are available in source; **this is not DRM and is not a legal-rights guarantee**.

For a simple source inspection, reviewers can read `gui_windows.go` (GUI), `core.go` (installation and validation), `build_payload.py` (format and generation), and `payload/manifest.json` (expected inputs and hashes).

## Security and privacy

- Offline installer: no networking, analytics, update check or telemetry.
- No privilege escalation and no background services.
- Refuses wrong/missing source files; validates source and output SHA-256.
- Does not overwrite originals or incompatible existing outputs.
- Unsigned Windows executable; Windows SmartScreen may warn.

## Reproducibility caveat

The repository is sufficient to **build the program with its checked-in patch payload**. Regenerating `payload/*.rvp` from scratch also requires the original and modified ZIPs, which are deliberately not redistributed. The encrypted payload generation uses a random nonce, so regenerating can produce different ciphertext despite identical reconstructed output files. Different Go compiler versions may also produce different EXE hashes.

## License and permissions

No rights to third-party game assets or to the original mod are claimed. Verify the upstream mod author's distribution/modification terms before publishing this repository or its patch payload. No open-source license has been assigned to this source code by default; the author can choose a license separately.
