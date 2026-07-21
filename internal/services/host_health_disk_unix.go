//go:build unix

package services

import "golang.org/x/sys/unix"

func fillDiskRoot(h *HostHealth) {
	var st unix.Statfs_t
	if err := unix.Statfs("/", &st); err != nil {
		return
	}
	// Bsize is portable across Linux/Darwin; cast carefully (int64 vs uint64 by OS)
	bsize := uint64(st.Bsize)
	if bsize == 0 {
		return
	}
	total := uint64(st.Blocks) * bsize
	// free for unprivileged: Bavail
	free := uint64(st.Bavail) * bsize
	if free > total {
		free = total
	}
	used := total - free
	h.DiskTotalBytes = total
	h.DiskUsedBytes = used
	if total > 0 {
		h.DiskUsedPct = float64(used) / float64(total) * 100
	}
}
