// SPDX-License-Identifier: MIT

//go:build windows

package creds

import (
	"syscall"
	"unsafe"
)

// keyWOW6464Key forces the 64-bit registry view so a (hypothetical) 32-bit
// build reads the same MachineGuid as a 64-bit one.
const keyWOW6464Key = 0x0100

// machineID returns the Windows installation's MachineGuid — written once by
// Windows setup at HKLM\SOFTWARE\Microsoft\Cryptography and stable for the OS
// install's lifetime. Read via the stdlib syscall registry bindings (no
// x/sys dependency, per the lean-deps policy). "" on any failure.
func machineID() string {
	subkey, err := syscall.UTF16PtrFromString(`SOFTWARE\Microsoft\Cryptography`)
	if err != nil {
		return ""
	}
	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, subkey, 0,
		syscall.KEY_READ|keyWOW6464Key, &h) != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)

	name, err := syscall.UTF16PtrFromString("MachineGuid")
	if err != nil {
		return ""
	}
	var typ uint32
	var buf [256]uint16
	n := uint32(len(buf) * 2) // byte count
	if syscall.RegQueryValueEx(h, name, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return ""
	}
	const regSZ = 1
	if typ != regSZ || n < 2 {
		return ""
	}
	return syscall.UTF16ToString(buf[: n/2 : n/2])
}
