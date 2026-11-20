module github.com/TaylorFinklea/harness-deck

go 1.26.3

// Release and CI builds use this toolchain (actions/setup-go honors it), so
// shipped binaries carry current standard-library security fixes. Bump it
// when Go ships a patch release; CI's govulncheck job flags a stale one.
toolchain go1.27.4
