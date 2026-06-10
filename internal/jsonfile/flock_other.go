//go:build !unix

package jsonfile

// lockFile is a no-op on platforms without flock (the project only ships
// darwin/linux binaries; `go install` on other platforms compiles but
// foregoes cross-process serialization, matching the pre-lock behavior).
func lockFile(string) (func(), error) {
	return func() {}, nil
}
