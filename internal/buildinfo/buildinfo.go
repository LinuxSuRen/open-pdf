// Package buildinfo carries version metadata injected at build time.
package buildinfo

// Version is the semantic version of the build ("dev" for source builds).
var Version = "dev"

// Commit is the git commit hash the binary was built from.
var Commit = "none"

// Date is the build timestamp (RFC3339).
var Date = "unknown"
