//go:build !unix && !windows

package dispatched

import (
	"fmt"
	"os"
)

func openSourceFileNoFollow(_ *os.File, _ string) (*os.File, error) {
	return nil, fmt.Errorf("atomic final-component source opens are unsupported on this platform")
}

func openSourceDirNoFollow(_ *os.File, _ string) (*os.File, error) {
	return nil, fmt.Errorf("atomic source directory opens are unsupported on this platform")
}

func clearSourceFileNonblock(_ *os.File) error {
	return nil
}

func sourceFileReadReady(_ *os.File) (bool, error) {
	return false, fmt.Errorf("bounded pipe readiness is unsupported on this platform")
}
