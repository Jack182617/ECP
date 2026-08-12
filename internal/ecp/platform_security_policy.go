package ecp

func authorityPlatformRequirement(supportsPrivateAuthority bool) error {
	if supportsPrivateAuthority {
		return nil
	}
	return newError(KindBlocked, "PLATFORM_SECURITY_UNSUPPORTED", "ECP v0.3 authority state requires POSIX private-file semantics; this platform is rejected before project or authority writes", nil)
}
