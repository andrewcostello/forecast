//go:build windows

package dispatched

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var sourcePeekNamedPipe = windows.NewLazySystemDLL("kernel32.dll").NewProc("PeekNamedPipe")

func openSourceHandleNoFollow(parent *os.File, name string, directory bool) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	conn, err := parent.SyscallConn()
	if err != nil {
		return nil, err
	}
	var opened windows.Handle
	var openErr error
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	if err := conn.Control(func(parentHandle uintptr) {
		attributes := windows.OBJECT_ATTRIBUTES{
			Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
			RootDirectory: windows.Handle(parentHandle),
			ObjectName:    objectName,
			Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		}
		var status windows.IO_STATUS_BLOCK
		openErr = windows.NtCreateFile(
			&opened,
			windows.FILE_GENERIC_READ,
			&attributes,
			&status,
			nil,
			0,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			windows.FILE_OPEN,
			options,
			0,
			0,
		)
	}); err != nil {
		return nil, err
	}
	if openErr != nil {
		if status, ok := openErr.(windows.NTStatus); ok {
			openErr = status.Errno()
		}
		return nil, &os.PathError{Op: "open", Path: name, Err: openErr}
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(opened, &info); err != nil {
		_ = windows.CloseHandle(opened)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(opened)
		return nil, fmt.Errorf("reparse points are not source files")
	}
	file := os.NewFile(uintptr(opened), name)
	if file == nil {
		_ = windows.CloseHandle(opened)
		return nil, os.ErrInvalid
	}
	return file, nil
}

// openSourceDirNoFollow uses the preceding held directory handle as
// RootDirectory and rejects directory reparse points before returning a handle
// that can anchor the next component.
func openSourceDirNoFollow(parent *os.File, name string) (*os.File, error) {
	return openSourceHandleNoFollow(parent, name, true)
}

// openSourceFileNoFollow opens only the final component relative to its held
// confined parent and rejects reparse points before ownership passes to os.File.
func openSourceFileNoFollow(parent *os.File, name string) (*os.File, error) {
	return openSourceHandleNoFollow(parent, name, false)
}

func clearSourceFileNonblock(_ *os.File) error {
	return nil
}

// sourceFileReadReady peeks without consuming bytes while SyscallConn keeps
// the pipe handle alive. A broken/disconnected pipe is ready for an EOF read.
func sourceFileReadReady(file *os.File) (bool, error) {
	conn, err := file.SyscallConn()
	if err != nil {
		return false, err
	}
	var available uint32
	var peekErr error
	ready := false
	if err := conn.Control(func(handle uintptr) {
		result, _, callErr := sourcePeekNamedPipe.Call(
			handle,
			0,
			0,
			0,
			uintptr(unsafe.Pointer(&available)),
			0,
		)
		if result != 0 {
			ready = available > 0
			return
		}
		if callErr == windows.ERROR_BROKEN_PIPE || callErr == windows.ERROR_PIPE_NOT_CONNECTED || callErr == windows.ERROR_NO_DATA {
			ready = true
			return
		}
		peekErr = callErr
	}); err != nil {
		return false, err
	}
	return ready, peekErr
}
