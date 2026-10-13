//go:build windows

package testmaster

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	ntdll                        = syscall.NewLazyDLL("ntdll.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
	procNtResumeProcess          = ntdll.NewProc("NtResumeProcess")
)

const (
	createSuspended    = 0x00000004
	processTerminate   = 0x0001
	processSetQuota    = 0x0100
	processSuspendResm = 0x0800
)

// tree holds a child's whole process tree in a Job Object. The child starts
// suspended and joins the job before its first instruction runs, so nothing
// it spawns can start outside the job. The job has no kill-on-close limit:
// like a Unix process group, it is killed only on timeout or interrupt, and a
// child a passing test leaves running keeps running.
type tree struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	job syscall.Handle
}

func prepareTree(cmd *exec.Cmd) *tree {
	t := &tree{cmd: cmd}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createSuspended}
	if isCmdExe(cmd.Args[0]) && len(cmd.Args) > 1 {
		// cmd.exe does not parse the quoting Go writes for argv, so the
		// command it runs goes on its command line verbatim.
		quoted := make([]string, len(cmd.Args)-1)
		for i, a := range cmd.Args[:len(cmd.Args)-1] {
			quoted[i] = syscall.EscapeArg(a)
		}
		cmd.SysProcAttr.CmdLine = strings.Join(quoted, " ") + " " + cmd.Args[len(cmd.Args)-1]
	}
	cmd.Cancel = t.kill
	return t
}

func isCmdExe(argv0 string) bool {
	base := strings.ToLower(argv0[strings.LastIndexAny(argv0, `\/`)+1:])
	return base == "cmd" || base == "cmd.exe"
}

// started puts the suspended child into a new job, then lets it run. A child
// that cannot join a job still runs; kill then falls back to taskkill /T.
func (t *tree) started() error {
	pid := uint32(t.cmd.Process.Pid)
	h, err := syscall.OpenProcess(processTerminate|processSetQuota|processSuspendResm, false, pid)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	if job, _, _ := procCreateJobObjectW.Call(0, 0); job != 0 {
		if ok, _, _ := procAssignProcessToJobObject.Call(job, uintptr(h)); ok != 0 {
			t.mu.Lock()
			t.job = syscall.Handle(job)
			t.mu.Unlock()
		} else {
			syscall.CloseHandle(syscall.Handle(job))
		}
	}
	if st, _, _ := procNtResumeProcess.Call(uintptr(h)); st != 0 {
		return syscall.Errno(st)
	}
	return nil
}

// kill ends every process in the tree.
func (t *tree) kill() error {
	t.mu.Lock()
	job := t.job
	t.mu.Unlock()
	if job != 0 {
		if ok, _, _ := procTerminateJobObject.Call(uintptr(job), uintptr(killedExit)); ok != 0 {
			return nil
		}
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(t.cmd.Process.Pid)).Run()
	return t.cmd.Process.Kill()
}

func (t *tree) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		syscall.CloseHandle(t.job)
		t.job = 0
	}
}
