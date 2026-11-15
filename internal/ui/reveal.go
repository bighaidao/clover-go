package ui

import (
	"github.com/bighaidao/clover-go/internal/app"
	"github.com/bighaidao/clover-go/internal/macos"
	"github.com/egoist/mygo"
)

// revealInFinderOrSelf implements the "在访达中显示" menu entry.
//
// The entry always means the real Finder: the user picked it precisely because
// they want to leave CloverGo. That is why it does not go through
// NSWorkspace.activateFileViewerSelectingURLs: (what mygo.Shell.ShowItemInFolder
// calls) — once CloverGo has taken over, NSFileViewer resolves that request
// back to CloverGo itself, and the menu appears to do nothing.
//
// Instead the path is handed to Finder.app directly, which bypasses the
// NSFileViewer lookup. When CloverGo has not taken over, the plain system call
// already opens Finder and is kept as-is.
func revealInFinderOrSelf(coord *app.Coordinator, path string) {
	if path == "" {
		return
	}
	if coord != nil && coord.Prefs != nil && coord.Prefs.Get().TakeoverFinder {
		if err := macos.RevealInRealFinder(path); err != nil {
			coord.Toast("无法在访达中显示: " + err.Error())
		}
		return
	}
	mygo.Shell.ShowItemInFolder(path)
}

// revealDirectoryOrSelf opens a directory in the real Finder, or in CloverGo
// when Finder is not the viewer. Used by the "在访达中打开" button on a failed
// directory load, where the target is a folder rather than an item.
func revealDirectoryOrSelf(coord *app.Coordinator, dir string) {
	if dir == "" {
		return
	}
	if coord != nil && coord.Prefs != nil && coord.Prefs.Get().TakeoverFinder {
		if err := macos.RevealInRealFinder(dir); err != nil {
			coord.Toast("无法在访达中打开: " + err.Error())
		}
		return
	}
	mygo.Shell.ShowItemInFolder(dir)
}
