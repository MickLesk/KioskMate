//go:build windows

package supervisor

import (
	"errors"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Edge and Chrome start a tree of GPU, network and renderer processes. Each
// browser started by KioskMate is put into a Job Object that kills the whole
// tree when it is closed: on a stop, after the main process exited, and when
// KioskMate itself ends. Stray children would otherwise keep the profile
// locked and the next start would only hand its URL to the old instance.

const gracefulStopTimeout = 3 * time.Second

var (
	processJobsMu sync.Mutex
	processJobs   = map[int]windows.Handle{}
)

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// adoptProcessTree puts the freshly started browser into a kill-on-close Job
// Object. Children started afterwards join the job automatically. A failure
// is not fatal: terminateProcessTree falls back to taskkill /T.
func adoptProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(process)
		_ = windows.CloseHandle(job)
		return
	}
	processJobsMu.Lock()
	processJobs[pid] = job
	processJobsMu.Unlock()
	go func() {
		// When the main browser process ends for any reason, remove the
		// processes it left behind.
		_, _ = windows.WaitForSingleObject(process, windows.INFINITE)
		_ = windows.CloseHandle(process)
		closeProcessJob(pid)
	}()
}

// closeProcessJob terminates every process left in the job of pid and
// releases it. It reports whether a job was known for pid.
func closeProcessJob(pid int) bool {
	processJobsMu.Lock()
	job, ok := processJobs[pid]
	delete(processJobs, pid)
	processJobsMu.Unlock()
	if !ok {
		return false
	}
	_ = windows.TerminateJobObject(job, 1)
	_ = windows.CloseHandle(job)
	return true
}

// terminateProcessTree first asks the browser to close its windows (taskkill
// without /F sends WM_CLOSE, so the profile is closed cleanly), waits up to
// gracefulStopTimeout and then ends the remaining processes.
func terminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = hiddenCommand("taskkill", "/T", "/PID", strconv.Itoa(pid)).Run()
	exited := waitProcessExit(pid, gracefulStopTimeout)
	if closeProcessJob(pid) || exited {
		return nil
	}
	err := hiddenCommand("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	if err != nil && waitProcessExit(pid, time.Second) {
		return nil
	}
	return err
}

// waitProcessExit reports whether pid is gone within timeout. A process that
// cannot be opened any more counts as gone.
func waitProcessExit(pid int, timeout time.Duration) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(process)
	event, err := windows.WaitForSingleObject(process, uint32(timeout.Milliseconds()))
	return err == nil && event == windows.WAIT_OBJECT_0
}

// hiddenCommand runs a console helper without flashing a console window.
func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}
