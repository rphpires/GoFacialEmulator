package utils

import (
	"runtime"
)

// Constants for controller types
var (
	DAHUA_CONTROLLER_TYPES = []int{
		22111, // DHI-ASI7213X-T1
		22121, // DHI-ASI7213Y-V3
		22131, // DHI-ASI7214Y-V3
	}

	HIKVISION_CONTROLLER_TYPES = []int{
		21101, // DS-K1T671
		21102, // DS-K1T673
	}
)

// Emulator executable paths
var (
	EMULATOR_DIST_PATH = "dist"
	EMULATOR_BASE_FILE = getEmulatorFileName()
)

// getEmulatorFileName returns the emulator executable file name based on the OS
func getEmulatorFileName() string {
	if runtime.GOOS == "windows" {
		return "facial_emulator_win.exe"
	}
	return "facial_emulator_unix"
}
