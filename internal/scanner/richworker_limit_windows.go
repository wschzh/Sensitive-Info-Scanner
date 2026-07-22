//go:build windows

package scanner

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	jobObjectInfoClassExtendedLimit = 9
	jobObjectLimitProcessMemory     = 0x00000100
	jobObjectLimitKillOnJobClose    = 0x00002000
	processSetQuota                 = 0x0100
	processTerminate                = 0x0001
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW        = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob      = kernel32.NewProc("AssignProcessToJobObject")
	procCloseHandle             = kernel32.NewProc("CloseHandle")
)

type jobObjectBasicLimitInformation struct {
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

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

func attachProcessLimit(p *os.Process, memoryLimitMB int) (func(), error) {
	if p == nil {
		return nil, nil
	}
	h, _, err := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return nil, fmt.Errorf("CreateJobObjectW: %w", err)
	}
	cleanup := func() { _, _, _ = procCloseHandle.Call(h) }
	limit := jobObjectExtendedLimitInformation{}
	limit.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	if memoryLimitMB > 0 {
		limit.BasicLimitInformation.LimitFlags |= jobObjectLimitProcessMemory
		limit.ProcessMemoryLimit = uintptr(memoryLimitMB) * 1024 * 1024
	}
	ok, _, err := procSetInformationJobObject.Call(
		h,
		uintptr(jobObjectInfoClassExtendedLimit),
		uintptr(unsafe.Pointer(&limit)),
		unsafe.Sizeof(limit),
	)
	if ok == 0 {
		cleanup()
		return nil, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	ph, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(p.Pid))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("OpenProcess: %w", err)
	}
	defer syscall.CloseHandle(ph)
	ok, _, err = procAssignProcessToJob.Call(h, uintptr(ph))
	if ok == 0 {
		cleanup()
		return nil, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return cleanup, nil
}
