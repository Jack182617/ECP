# ECP Distribution Contract

The supported ordinary-user unit is one Codex Plugin containing four focused
ECP Skills for check, enable, disable, and governed change; one shared
Plugin-root launcher; one shared CLI contract; and compatible local Core
runtimes. Ordinary users must not install a separate ECP CLI, modify PATH, copy
protocol identifiers, or select a runtime architecture.

Every Skill must resolve the same launcher from its installed Plugin path. The
launcher must select only a packaged supported OS/architecture, reject
missing/unsafe artifacts, verify the selected binary against its package
checksum, and fail before repository writes when it cannot establish a valid
runtime. It must never fall back to an ambient `ecp`, repository binary,
temporary build, or stale remembered path.

The generated runtime manifest must bind every artifact path, digest, size, and
supported target to the exact Plugin version, canonical source commit,
source-clean state, Go toolchain version/executable digest, and fixed build
flags. A formal qualification or release candidate requires clean committed
source; an explicit dirty override produces only a non-qualifying local
development artifact with `source_clean: false`. Core/Skill/Plugin changes
require a new cachebuster followed by a complete runtime rebuild and validation.

Installing, updating, rolling back, or removing the Plugin must not silently
register, enable, disable, migrate, repair, or delete any project or authority
state. A rollback is delivered as a reviewed source revert with a new Plugin
cachebuster and a newly validated package; operators must not hand-edit the
Plugin cache or select an old cached directory as an executable authority. If
the Plugin is removed while a Workspace remains enabled, that Workspace must
remain governed and unavailable for supported Codex mutations until a
compatible Plugin is reinstalled or the project is explicitly disabled before
uninstall.

Checksums establish package consistency, not publisher identity. A release may
be described as trusted distribution only after its binaries and Plugin package
are built by the designated release process, signed/notarized where applicable,
installed through an intended marketplace, and verified in a fresh desktop
task. Installing the Plugin never enables ECP for a project.
