// Package buildinfo carries build-time metadata such as the release version.
package buildinfo

// Version is set at link time by the Makefile (-ldflags -X) and reported in the
// protocol hello message (docs/PROTOCOL.md). Golden tests replace it with "test".
var Version = "dev"
