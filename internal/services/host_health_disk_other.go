//go:build !unix

package services

func fillDiskRoot(h *HostHealth) {
	// Non-unix: leave disk metrics zero
}
