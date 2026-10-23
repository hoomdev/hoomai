//go:build windows

package procesos

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll") // nosemgrep: trailofbits.go.unsafe-dll-loading.unsafe-dll-loading -- kernel32.dll esta en KnownDLLs: Windows siempre la carga de System32
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
	processSetQuota                   = 0x0100
	processTerminate                  = 0x0001
)

// the layout of JOBOBJECT_EXTENDED_LIMIT_INFORMATION; Go aligns the fields
// as the C compiler does
type jobBasicLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobIoCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type jobExtendedLimits struct {
	Basic                 jobBasicLimits
	Io                    jobIoCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// jobs are the Job Objects of the processes adopted, by pid.
var jobs = struct {
	sync.Mutex
	porPID map[int]syscall.Handle
}{porPID: map[int]syscall.Handle{}}

// EnGrupo makes ending cmd (exec's Cancel, when its context ends) kill the
// whole tree it started: its Job Object when Adoptar put it in one, else
// taskkill /T.
func EnGrupo(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return MatarGrupo(cmd.Process.Pid) }
}

// Adoptar puts the process cmd just started in a Job Object of its own that
// kills every process in it when it is terminated, and when its last handle
// closes: if this process dies without killing it, the tree dies too. What
// the process launches from then on is in the job. If Windows does not let
// it (a process already in a job that does not nest), the tree stays without
// a job and MatarGrupo falls back to taskkill /T.
func Adoptar(cmd *exec.Cmd) {
	pid := cmd.Process.Pid
	job, _, _ := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		return
	}
	var info jobExtendedLimits
	info.Basic.LimitFlags = jobObjectLimitKillOnJobClose
	if r, _, _ := procSetInformationJobObject.Call(job, jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); r == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return
	}
	proc, _, _ := procOpenProcess.Call(processSetQuota|processTerminate, 0, uintptr(pid))
	if proc == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return
	}
	defer syscall.CloseHandle(syscall.Handle(proc))
	if r, _, _ := procAssignProcessToJobObject.Call(job, proc); r == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return
	}
	jobs.Lock()
	jobs.porPID[pid] = syscall.Handle(job)
	jobs.Unlock()
}

// Soltar closes the Job Object of pid, once its process has ended.
func Soltar(pid int) {
	jobs.Lock()
	job, ok := jobs.porPID[pid]
	delete(jobs.porPID, pid)
	jobs.Unlock()
	if ok {
		syscall.CloseHandle(job)
	}
}

// MatarGrupo kills the tree of pid: its Job Object, else taskkill /T /F.
func MatarGrupo(pid int) error {
	if pid <= 0 {
		return nil
	}
	jobs.Lock()
	job, ok := jobs.porPID[pid]
	jobs.Unlock()
	if ok {
		if r, _, err := procTerminateJobObject.Call(uintptr(job), 1); r == 0 {
			return err
		}
		return nil
	}
	// taskkill has its own bound: exec calls Cancel before WaitDelay starts,
	// so a taskkill that hangs would hold whoever is cancelling (finding
	// 3f39e2)
	ctx, cancel := context.WithTimeout(context.Background(), plazoTaskkill)
	defer cancel()
	return exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

// plazoTaskkill bounds the taskkill of MatarGrupo.
const plazoTaskkill = 5 * time.Second

// MismoProceso answers true on Windows: a reused pid is not told apart there
// (the risk the spec accepts for a recycled PID).
func MismoProceso(pid int, arranco time.Time) bool { return pid > 0 }

// Senales are the signals after which this process must not leave its trees
// behind: Ctrl-C and Ctrl-Break (os.Interrupt), and closing the console,
// logging off or shutting down (SIGTERM; Windows ends the process shortly
// after, so the handler only kills and leaves).
var Senales = []os.Signal{os.Interrupt, syscall.SIGTERM}

// Terminar ends this process once its trees are dead: Windows has no way to
// re-send the signal to itself.
func Terminar(s os.Signal) {
	os.Exit(1)
}
