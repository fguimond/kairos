// Package version exposes the kairos build version.
//
// Version is set to "development" by default and overridden at build time via
// -ldflags="-X github.com/francisguimond/kairos/internal/version.Version=<tag>".
package version

var Version = "development"
