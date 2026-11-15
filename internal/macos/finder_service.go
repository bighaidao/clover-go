package macos

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	openFilesOnce sync.Once
	openFilesMu   sync.Mutex
	openFilesFn   func(path string)
)

// RegisterLegacyOpenFilesHook injects `application:openFiles:` and `application:openFile:`
// into `MyGoAppDelegate` at runtime via Objective-C class_addMethod.
//
// Why this is needed:
// MyGo's built-in `MyGoAppDelegate` only implements `application:openURLs:`.
// However, when macOS `NSFileViewer` is triggered via `[NSWorkspace selectFile:inFileViewerRootedAtPath:]`
// (used by Electron, Chrome, VSCode, WeChat, etc.), AppKit checks whether the NSApplicationDelegate
// responds to `application:openFiles:` / `application:openFile:` first, and skips `application:openURLs:`
// or drops the legacy AppleEvent when those selectors are missing.
func RegisterLegacyOpenFilesHook(onOpen func(path string)) {
	openFilesMu.Lock()
	openFilesFn = onOpen
	openFilesMu.Unlock()

	openFilesOnce.Do(func() {
		libObjC, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil || libObjC == 0 {
			return
		}
		classAddMethodAddr, err := purego.Dlsym(libObjC, "class_addMethod")
		if err != nil || classAddMethodAddr == 0 {
			return
		}
		var classAddMethod func(cls objc.Class, name objc.SEL, imp uintptr, types *byte) bool
		purego.RegisterFunc(&classAddMethod, classAddMethodAddr)

		cls := objc.GetClass("MyGoAppDelegate")
		if cls == 0 {
			return
		}

		selCount := objc.RegisterName("count")
		selObjectAtIndex := objc.RegisterName("objectAtIndex:")
		selUTF8String := objc.RegisterName("UTF8String")
		selReply := objc.RegisterName("replyToOpenOrPrint:")

		nsStrToGo := func(nsStr objc.ID) string {
			if nsStr == 0 {
				return ""
			}
			ptr := objc.Send[*byte](nsStr, selUTF8String)
			if ptr == nil {
				return ""
			}
			var n int
			for *(*byte)(unsafe.Add(unsafe.Pointer(ptr), n)) != 0 {
				n++
			}
			return string(unsafe.Slice(ptr, n))
		}

		// 1. -(void)application:(NSApplication *)sender openFiles:(NSArray<NSString *> *)filenames
		impOpenFiles := purego.NewCallback(func(self objc.ID, _cmd objc.SEL, sender objc.ID, filenames objc.ID) {
			if filenames != 0 {
				count := objc.Send[int](filenames, selCount)
				for i := 0; i < count; i++ {
					item := objc.Send[objc.ID](filenames, selObjectAtIndex, i)
					p := nsStrToGo(item)
					if p != "" {
						openFilesMu.Lock()
						fn := openFilesFn
						openFilesMu.Unlock()
						if fn != nil {
							fn(p)
						}
					}
				}
			}
			if sender != 0 {
				// NSApplicationDelegateReplySuccess = 0
				sender.Send(selReply, 0)
			}
		})
		typesOpenFiles := append([]byte("v@:@@"), 0)
		classAddMethod(cls, objc.RegisterName("application:openFiles:"), impOpenFiles, &typesOpenFiles[0])

		// 2. -(BOOL)application:(NSApplication *)sender openFile:(NSString *)filename
		impOpenFile := purego.NewCallback(func(self objc.ID, _cmd objc.SEL, sender objc.ID, filename objc.ID) uintptr {
			p := nsStrToGo(filename)
			if p != "" {
				openFilesMu.Lock()
				fn := openFilesFn
				openFilesMu.Unlock()
				if fn != nil {
					fn(p)
				}
				return 1
			}
			return 0
		})
		typesOpenFile := append([]byte("B@:@@"), 0)
		classAddMethod(cls, objc.RegisterName("application:openFile:"), impOpenFile, &typesOpenFile[0])
	})
}

// ParseCloverURL parses fclover:// URL schemes.
// Supports: fclover://open?path=/Users/... and fclover://reveal?path=...
func ParseCloverURL(rawURL string) (action, targetPath string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "fclover" {
		return "", "", fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}

	action = strings.ToLower(u.Host)
	if action == "" {
		action = strings.TrimPrefix(u.Path, "/")
	}

	q := u.Query()
	targetPath = q.Get("path")
	if targetPath == "" && u.Path != "" && u.Host != "open" && u.Host != "reveal" {
		targetPath = filepath.Clean(u.Host + u.Path)
	}

	return action, targetPath, nil
}

// resolveAppBundlePath returns the path to CloverGo.app bundle.
func resolveAppBundlePath() string {
	if exe, err := os.Executable(); err == nil {
		// /path/to/CloverGo.app/Contents/MacOS/CloverGo
		bundle := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
		if strings.HasSuffix(bundle, ".app") {
			return bundle
		}
	}
	return "/Applications/CloverGo.app"
}

// SetFinderTakeover enables or disables macOS Finder takeover (NSFileViewer & public.folder).
func SetFinderTakeover(enable bool) error {
	mode := "disable"
	args := []string{mode}
	if enable {
		mode = "enable"
		args = []string{mode, resolveAppBundlePath()}
	}
	_, err := runHelper("finder-takeover", args...)
	return err
}

// IsFinderTakeoverEnabled queries the current macOS system NSFileViewer status.
func IsFinderTakeoverEnabled() bool {
	out, err := runHelper("finder-takeover", "status")
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(out), "ENABLED")
}

// HelperPath returns the absolute path to the bundled clover-helper binary.
// It prefers the sibling of the running executable (inside CloverGo.app/Contents/MacOS),
// then the development build directory.
func HelperPath() string {
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "clover-helper")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, "build", "clover-helper")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// watchAgentBundlePath returns the nested CloverGoWatch.app path inside the main bundle.
func watchAgentBundlePath() string {
	if exe, err := os.Executable(); err == nil {
		bundle := filepath.Join(filepath.Dir(exe), "..", "Helpers", "CloverGoWatch.app")
		bundle = filepath.Clean(bundle)
		if _, err := os.Stat(bundle); err == nil {
			return bundle
		}
	}
	return ""
}

var (
	finderWatchMu   sync.Mutex
	finderWatchProc *os.Process
)

// StartFinderWatch launches the background Finder interception agent.
// It mirrors NewFinder's WatchAgent: a dedicated nested .app with its own bundle
// identifier so LaunchServices and TCC treat it as an independent agent.
func StartFinderWatch() {
	finderWatchMu.Lock()
	defer finderWatchMu.Unlock()

	if finderWatchProc != nil {
		if finderWatchProc.Signal(syscall.Signal(0)) == nil {
			return // already running
		}
		finderWatchProc = nil
	}

	// Preferred: the nested CloverGoWatch.app bundle.
	if bundle := watchAgentBundlePath(); bundle != "" {
		exe := filepath.Join(bundle, "Contents", "MacOS", "CloverGoWatch")
		if _, statErr := os.Stat(exe); statErr == nil {
			proc, err := os.StartProcess(exe, []string{exe, "finder-watch", strconv.Itoa(os.Getpid())}, &os.ProcAttr{
				Dir:   "/",
				Files: []*os.File{nil, nil, nil},
			})
			if err == nil {
				finderWatchProc = proc
				go func() { _, _ = proc.Wait() }()
				return
			}
		}
	}

	// Fallback: run the helper subcommand directly.
	if helper := HelperPath(); helper != "" {
		cmd := exec.Command(helper, "finder-watch", strconv.Itoa(os.Getpid()))
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err == nil {
			finderWatchProc = cmd.Process
			go func() { _ = cmd.Wait() }()
		}
	}
}

// StopFinderWatch terminates the background interception agent if running.
func StopFinderWatch() {
	finderWatchMu.Lock()
	defer finderWatchMu.Unlock()
	if finderWatchProc != nil {
		_ = finderWatchProc.Kill()
		finderWatchProc = nil
	}
	// Also sweep any orphaned agent processes.
	_ = exec.Command("/usr/bin/pkill", "-f", "CloverGoWatch").Run()
	_ = exec.Command("/usr/bin/pkill", "-f", "finder-watch").Run()
}

// InstallWatchLaunchAgent writes a LaunchAgent so the interception agent survives
// a CloverGo quit and can revive the UI when the user clicks the Finder icon.
func InstallWatchLaunchAgent() error {
	bundle := watchAgentBundlePath()
	if bundle == "" {
		return fmt.Errorf("CloverGoWatch.app not found in bundle")
	}
	exe := filepath.Join(bundle, "Contents", "MacOS", "CloverGoWatch")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("CloverGoWatch executable missing: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agentsDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return err
	}
	label := "com.clover.app.watch"
	plistPath := filepath.Join(agentsDir, label+".plist")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>finder-watch</string>
        <string>0</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`, label, exe)
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}

	_ = exec.Command("/bin/launchctl", "unload", plistPath).Run()
	return exec.Command("/bin/launchctl", "load", plistPath).Run()
}

// UninstallWatchLaunchAgent removes the LaunchAgent installed by InstallWatchLaunchAgent.
func UninstallWatchLaunchAgent() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.clover.app.watch.plist")
	_ = exec.Command("/bin/launchctl", "unload", plistPath).Run()
	_ = os.Remove(plistPath)
}

// RevealInRealFinder opens the system Finder and selects path in it, even when
// CloverGo currently owns the file viewer role.
//
// NSWorkspace.activateFileViewerSelectingURLs: and mygo.Shell.ShowItemInFolder
// both resolve through NSFileViewer, so once CloverGo has taken over they come
// straight back to CloverGo. Launching Finder.app explicitly with an Apple
// Event bypasses that lookup: Finder is told to reveal the path itself.
//
// The path may be a file, a folder, or a .app bundle; Finder selects whatever
// it is given inside its parent folder, which is what "Show in Finder" means.
func RevealInRealFinder(path string) error {
	if path == "" {
		return fmt.Errorf("reveal: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if _, statErr := os.Stat(abs); statErr != nil {
		return fmt.Errorf("reveal: %w", statErr)
	}

	// Prefer the helper, which talks to Finder through NSWorkspace's own
	// reveal API while explicitly targeting Finder.app, so the request cannot
	// be routed back to CloverGo.
	if _, err := runHelper("finder-reveal", abs); err == nil {
		return nil
	}

	// Fallback: drive Finder with AppleScript. Works without the helper, and
	// still bypasses NSFileViewer because the target app is named explicitly.
	script := fmt.Sprintf(
		`tell application "Finder" to reveal (POSIX file %q as alias)`,
		abs,
	)
	cmd := exec.Command("/usr/bin/osascript", "-e", script)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reveal in Finder: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ActivateFinder brings the real Finder to the front, used after revealing so
// the selected item is visible rather than hidden behind CloverGo.
func ActivateFinder() {
	_ = exec.Command("/usr/bin/open", "-a", "Finder").Run()
}
