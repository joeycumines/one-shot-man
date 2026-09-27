package node

import "io/fs"

func nodeStatMode(mode fs.FileMode) uint32 {
	fileType := uint32(0o100000)
	switch {
	case mode&fs.ModeSymlink != 0:
		fileType = 0o120000
	case mode&fs.ModeDir != 0:
		fileType = 0o040000
	case mode&fs.ModeNamedPipe != 0:
		fileType = 0o010000
	case mode&fs.ModeSocket != 0:
		fileType = 0o140000
	case mode&fs.ModeDevice != 0 && mode&fs.ModeCharDevice != 0:
		fileType = 0o020000
	case mode&fs.ModeDevice != 0:
		fileType = 0o060000
	}

	permissions := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		permissions |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		permissions |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		permissions |= 0o1000
	}
	return fileType | permissions
}
