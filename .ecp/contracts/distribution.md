# ECP Distribution Contract

The supported ordinary-user unit is one Codex Plugin containing the ECP Skill,
its skill-relative launcher, and compatible local Core runtimes. Ordinary users
must not install a separate ECP CLI, modify PATH, copy protocol identifiers, or
select a runtime architecture.

The launcher must resolve from the installed Skill path, select only a packaged
supported OS/architecture, reject missing/unsafe artifacts, verify the selected
binary against its package checksum, and fail before repository writes when it
cannot establish a valid runtime. It must never fall back to an ambient `ecp`,
repository binary, temporary build, or stale remembered path.

The generated runtime manifest must bind every artifact path, digest, size, and
supported target to the exact Plugin version. Core/Skill/Plugin changes require
a new cachebuster followed by a complete runtime rebuild and validation.

Checksums establish package consistency, not publisher identity. A release may
be described as trusted distribution only after its binaries and Plugin package
are built by the designated release process, signed/notarized where applicable,
installed through an intended marketplace, and verified in a fresh desktop
task. Installing the Plugin never enables ECP for a project.
