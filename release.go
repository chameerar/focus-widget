//go:build production

package main

// release is true in builds made with -tags production, which is how installers are built.
const release = true
