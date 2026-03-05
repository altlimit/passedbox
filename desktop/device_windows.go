//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

type DevicePepperInfo struct {
	Available   bool   `json:"available"`
	SerialID    string `json:"serialId"`
	IsRemovable bool   `json:"isRemovable"`
}

func getDevicePepperInfoOS() DevicePepperInfo {
	// Get current working directory (drive letter)
	wd, err := os.Getwd()
	if err != nil {
		return DevicePepperInfo{Available: false}
	}

	vol := filepath.VolumeName(wd)
	if vol == "" {
		return DevicePepperInfo{Available: false}
	}

	// Add backslash for Windows API. e.g. "C:\"
	rootPath := vol + "\\"

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getVolumeInformation := kernel32.NewProc("GetVolumeInformationW")
	getDriveType := kernel32.NewProc("GetDriveTypeW")

	rootNamePtr, _ := syscall.UTF16PtrFromString(rootPath)

	var serialNumber uint32

	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumeinformationw
	ret, _, _ := getVolumeInformation.Call(
		uintptr(unsafe.Pointer(rootNamePtr)),
		0, 0,
		uintptr(unsafe.Pointer(&serialNumber)),
		0, 0, 0, 0,
	)

	if ret == 0 {
		return DevicePepperInfo{Available: false}
	}

	driveTypeRet, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(rootNamePtr)))

	// DRIVE_REMOVABLE is 2
	isRemovable := driveTypeRet == 2

	// Format serial number as 8 hex digits
	serialHex := fmt.Sprintf("%08X", serialNumber)
	// Often displayed as XXXX-XXXX
	formattedSerial := fmt.Sprintf("%s-%s", serialHex[:4], serialHex[4:])

	return DevicePepperInfo{
		Available:   true,
		SerialID:    formattedSerial,
		IsRemovable: isRemovable,
	}
}
