//go:build darwin || linux

package dispatched

import (
	"os"

	"golang.org/x/sys/unix"
)

// openSourceFileNoFollow atomically opens one basename relative to a held
// confined-parent descriptor. os.Root.OpenFile cannot be used here because its
// Unix implementation deliberately follows ELOOP even when O_NOFOLLOW was
// supplied by its caller.
func openSourceFileNoFollow(parent *os.File, name string) (*os.File, error) {
	conn, err := parent.SyscallConn()
	if err != nil {
		return nil, err
	}
	opened := -1
	var openErr error
	if err := conn.Control(func(parentFD uintptr) {
		opened, openErr = unix.Openat(int(parentFD), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	}); err != nil {
		return nil, err
	}
	if openErr != nil {
		return nil, openErr
	}
	file := os.NewFile(uintptr(opened), name)
	if file == nil {
		_ = unix.Close(opened)
		return nil, os.ErrInvalid
	}
	return file, nil
}

// sourceFileReadReady holds the descriptor through SyscallConn while polling,
// so Close cannot recycle the descriptor under the readiness check. Poll does
// not alter the runtime-managed blocking mode.
func sourceFileReadReady(file *os.File) (bool, error) {
	conn, err := file.SyscallConn()
	if err != nil {
		return false, err
	}
	var ready bool
	var pollErr error
	if err := conn.Control(func(fd uintptr) {
		poll := []unix.PollFd{{
			Fd:     int32(fd),
			Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR,
		}}
		var n int
		n, pollErr = unix.Poll(poll, 100)
		if pollErr == unix.EINTR {
			pollErr = nil
			return
		}
		if pollErr == nil && n > 0 {
			if poll[0].Revents&unix.POLLNVAL != 0 {
				pollErr = os.ErrClosed
				return
			}
			ready = true
		}
	}); err != nil {
		return false, err
	}
	return ready, pollErr
}
