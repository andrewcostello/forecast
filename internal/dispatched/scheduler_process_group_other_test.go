//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package dispatched_test

import "os/exec"

func configureLargeCapCancellation(cmd *exec.Cmd) {
	// The probe runs CPU-only scheduler code and does not start descendants. On
	// non-POSIX targets, cancellation therefore kills the only child process;
	// POSIX builds additionally kill the child's process group defensively.
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
