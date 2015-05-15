package main

import fswatcher "github.com/rshmelev/go-simplified-fs-watcher/fswatcher"

func main() {
	folderToMonitor := "./"
	if !fswatcher.FileExists(folderToMonitor) {
		return
	}
	w := fswatcher.WatchDir(folderToMonitor)
	for {
		ev, ok := <-w.Events
		if !ok {
			break
		}
		println(ev.String())
	}

}
