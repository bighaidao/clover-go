package macos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LaunchAtLogin controls whether CloverGo starts automatically when the user
// logs in.
//
// The registration is a LaunchAgent in ~/Library/LaunchAgents rather than
// SMAppService.mainApp: the latter registers the bundle of the calling
// process, and the call goes through clover-helper, which is its own
// executable. A LaunchAgent works on every macOS version CloverGo supports
// (12+) and points straight at the app bundle.
const launchAtLoginLabel = "com.clover.app.loginitem"

// SetLaunchAtLogin enables or disables starting CloverGo at login.
func SetLaunchAtLogin(enable bool) error {
	mode := "disable"
	if enable {
		mode = "enable"
	}
	_, err := runHelper("launch-at-login", mode, resolveAppBundlePath())
	return err
}

// IsLaunchAtLoginEnabled reports whether CloverGo is currently registered to
// start at login.
func IsLaunchAtLoginEnabled() bool {
	out, err := runHelper("launch-at-login", "status")
	if err != nil {
		return false
	}
	return len(out) > 0 && out[0] == 'E'
}

// SyncLaunchAtLogin reconciles the login item with the stored preference, so
// that a preference changed while the helper was unavailable, or an app moved
// to another path, is corrected on the next launch.
func SyncLaunchAtLogin(enable bool) error {
	return SetLaunchAtLogin(enable)
}

// launchAtLoginPlistPath is where the agent lives.
func launchAtLoginPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAtLoginLabel+".plist"), nil
}

// RemoveLaunchAtLoginPlist deletes the agent without going through the helper.
// Used when the helper itself is missing, so a stale login item pointing at a
// deleted bundle cannot survive an uninstall.
func RemoveLaunchAtLoginPlist() {
	path, err := launchAtLoginPlistPath()
	if err != nil {
		return
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return
	}
	_ = exec.Command("/bin/launchctl", "unload", path).Run()
	_ = os.Remove(path)
}

// LaunchAtLoginSupported reports whether the helper needed to change the login
// item is available.
func LaunchAtLoginSupported() error {
	if HelperPath() == "" {
		return fmt.Errorf("clover-helper 未找到")
	}
	return nil
}
