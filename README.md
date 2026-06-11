# weldr-shim

A minimal Weldr API v1 server that wraps `image-builder` CLI to provide composer-cli (`weldr-client`) compatibility.

- **Filesystem-based state** - No database, all state stored as files in `/var/cache/weldr-shim`
- **Single binary** - No external dependencies beyond `image-builder` CLI
- **Compatibility** - Complatible with the official `weldr-client`
- **Unix socket API** - Compatible with existing composer-cli configuration
- **Auto-detection** - Finds either `image-builder` or legacy `image-builder-cli` binary
- **Subprocess execution** - Direct CLI wrapping with no image-builder internals

State management:

- Blueprints: `/var/cache/weldr-shim/blueprints/*.json`
- Composes: `/var/cache/weldr-shim/composes/<uuid>/`
- Image types cache: Loaded once at startup from `image-builder list --format json`

## Project status

This is an **experiment**. It was tested with Fedra 43 x86_64 qcow2 image only. Output artifacts are likely named differently.

## Building

```bash
make
```

## Usage

Requires root for /run/weldr/api.socket and some image builds.

```bash
sudo ./weldr-shim
```

Then simply use the `weldr-client`:

```bash
composer-cli status show
composer-cli blueprints push test.toml
composer-cli compose start test tar
```

## Environment Variables

- `MANIFEST_ONLY=1` - Use `image-builder manifest` instead of `build` (for fast testing). Make sure to use `sudo -E` to pass it on.
- `WELDR_DEFAULT_ARCH` - Override default architecture (default: auto-detected from system, e.g., `x86_64`, `aarch64`)
- `WELDR_DEFAULT_DISTRO` - Override default distro (default: auto-detected from `/etc/os-release`, e.g., `fedora-43`, `rhel-10`)

The default architecture is detected from the system's runtime architecture:
- `amd64` → `x86_64`
- `arm64` → `aarch64`
- `ppc64le` → `ppc64le`
- `s390x` → `s390x`

The default distro is detected from `/etc/os-release` (ID and VERSION_ID fields).

**Note:** Blueprints can specify their own `distro` and `architecture` fields. When present in the blueprint, these values take priority over the defaults. This allows per-blueprint control of target distribution and architecture.

## Testing

Smoke test executes all supported commands. Use `MANIFEST_ONLY` to speed up testing (does not actually build any image).

```bash
export MANIFEST_ONLY=1
make smoke-test
```

## State Directory

Default: `/var/cache/weldr-shim/`

Structure:
```
/var/cache/weldr-shim/
├── blueprints/       # Blueprint JSON files
├── composes/         # Compose directories
│   └── {uuid}/
│       ├── metadata.json
│       ├── status
│       ├── pid (when running)
│       └── result/   # Image output
└── store/            # image-builder-cli cache
```

## Implemented Commands

### Status
- ✅ `composer-cli status show` - Server status and version

### Blueprints
- ✅ `composer-cli blueprints list` - List all blueprints
- ✅ `composer-cli blueprints show <name>` - Show blueprint in TOML format
- ✅ `composer-cli blueprints push <file.toml>` - Create/update blueprint
- ✅ `composer-cli blueprints delete <name>` - Delete blueprint
- ❌ `composer-cli blueprints save <name>` - Not implemented
- ❌ `composer-cli blueprints changes <name>` - Not implemented (no versioning)
- ❌ `composer-cli blueprints diff <name> <from> <to>` - Not implemented (no versioning)
- ❌ `composer-cli blueprints freeze <name>` - Not implemented
- ❌ `composer-cli blueprints depsolve <name>` - Not implemented
- ❌ `composer-cli blueprints tag <name>` - Not implemented (no versioning)
- ❌ `composer-cli blueprints undo <name> <commit>` - Not implemented (no versioning)
- ❌ `composer-cli blueprints workspace <name>` - Not implemented

### Compose
- ✅ `composer-cli compose start <blueprint> <type>` - Start a compose
- ✅ `composer-cli compose types` - List available image types
- ✅ `composer-cli compose status` - List all composes with status
- ✅ `composer-cli compose list` - List composes (basic info)
- ✅ `composer-cli compose info <uuid>` - Show detailed compose info
- ✅ `composer-cli compose wait <uuid>` - Wait for compose to finish
- ✅ `composer-cli compose image <uuid>` - Download compose image
- ✅ `composer-cli compose cancel <uuid>` - Cancel running compose
- ✅ `composer-cli compose delete <uuid>` - Delete compose
- ❌ `composer-cli compose log <uuid>` - Not implemented
- ❌ `composer-cli compose logs <uuid>` - Not implemented
- ❌ `composer-cli compose metadata <uuid>` - Not implemented
- ❌ `composer-cli compose results <uuid>` - Not implemented
- ❌ `composer-cli compose start-ostree` - Not implemented

### Distros
- ✅ `composer-cli distros list` - List available distributions

### Not Implemented (Entire Categories)
- ❌ `composer-cli modules *` - All module commands
- ❌ `composer-cli projects *` - All project commands
- ❌ `composer-cli sources *` - All source management commands

## Supported Weldr API v1 Endpoints

```
GET  /api/status                      - Server status
GET  /api/v1/blueprints/list          - List blueprints
GET  /api/v1/blueprints/info/<name>   - Blueprint info
POST /api/v1/blueprints/new           - Create/update blueprint
DEL  /api/v1/blueprints/delete/<name> - Delete blueprint
GET  /api/v1/compose/types            - Image types
POST /api/v1/compose                  - Start compose
GET  /api/v1/compose/status/<uuid>    - Compose status
GET  /api/v1/compose/info/<uuid>      - Compose info
GET  /api/v1/compose/queue            - Queue status
GET  /api/v1/compose/finished         - Finished composes
GET  /api/v1/compose/failed           - Failed composes
GET  /api/v1/compose/image/<uuid>     - Download image
DEL  /api/v1/compose/delete/<uuid>    - Delete compose
DEL  /api/v1/compose/cancel/<uuid>    - Cancel compose
GET  /api/v1/distros/list             - List distros
```

All other Weldr API endpoints return HTTP 501 Not Implemented.

## Limitations

- No blueprint versioning (changes, diff, undo, tag)
- No workspace support
- No dependency resolution (depsolve, freeze)
- No module/project/source management
- No compose logs or metadata endpoints
- No ostree compose support
- Compose queue processes one at a time (no parallelism)

## Dependencies

- Go 1.23.9+
- `weldr-client`
- `image-builder` or `image-builder-cli` (must be in PATH)

