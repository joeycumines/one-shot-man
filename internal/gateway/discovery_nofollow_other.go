//go:build !unix

package gateway

// openNoFollow has no equivalent on platforms without O_NOFOLLOW (Windows,
// Plan 9): the discovery file is created with O_EXCL, which already refuses
// any pre-existing entry including a symlink, so the flag is simply absent.
const openNoFollow = 0
