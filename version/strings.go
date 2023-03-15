package version

import (
	"fmt"
)

// These are targets for compiling in build information.
// They are set by the Makefile and .goreleaser.yml

var (
	// Hash git commit hash.
	Hash string

	// ShortHash git commit short hash.
	ShortHash string

	// CompileTime of the binary.
	CompileTime string

	// ReleaseVersion based on release tag.
	ReleaseVersion string
)

// UnknownVersion is used when the version is not known.
const UnknownVersion = "(unknown version)"

// Version the binary version.
func Version() string {
	if ReleaseVersion == "" {
		return UnknownVersion
	}
	return ReleaseVersion
}

// LongVersion the long form of the binary version.
func LongVersion() string {
	if ReleaseVersion == "" || Hash == "" || CompileTime == "" {
		return UnknownVersion
	}

	return fmt.Sprintf("Conduit %s (%s)", ReleaseVersion, ShortHash)
}
