//go:build !darwin && !linux

package keys

func isDarwin() bool { return false }
func isLinux() bool  { return false }
