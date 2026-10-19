//go:build !windows && (!darwin || !cgo)

package platform

func setupNotify(onClick func(), onError func(string)) {}

func nativeNotify(title, message string) bool { return false }
