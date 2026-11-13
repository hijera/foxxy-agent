//go:build android && !cgo

package platform

// FoxxyCode for Android is built with cgo, linked by the NDK against Bionic (make
// android): Bionic resolves hostnames and hands a program the arguments the
// system linker was given, and a build without cgo gets neither - it would
// ask 127.0.0.1:53 for every host and read the linker's argument as its own.
// Go cross-compiles with cgo off by default, so this stops that build here.
const _ = android_build_needs_cgo_see_make_android
