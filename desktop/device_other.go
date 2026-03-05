//go:build !windows

package main

type DevicePepperInfo struct {
	Available   bool   `json:"available"`
	SerialID    string `json:"serialId"`
	IsRemovable bool   `json:"isRemovable"`
}

func getDevicePepperInfoOS() DevicePepperInfo {
	return DevicePepperInfo{
		Available: false,
	}
}
