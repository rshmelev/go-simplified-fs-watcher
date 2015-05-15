package advfswatcher

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/howeyc/fsnotify"
	. "github.com/rshmelev/go-inthandler"
)

/*

	bonuses:
	- it is a smart wrapper around github.com/howeyc/fsnotify
	- create+many modify events -> single create event when file is finally ready to use
	- simple file create-modified-deleted pattern across all OSes
	- super easy-to-use API
	- generates file created events on watching start
	- generates modification event when modification is finally finished, avoiding huge amount of intermid mod events
	- anyways the intention is to not send you the garbage (intermediate events)

	limitations:
	- updating child dir files may not result in Modify events on linux
	- file renaming may not result in "because of renaming" flag during the Created event on linux
	- dir deletion may not result in "because of renaming" flag during the Deleted event on Win
	- some dir moving inside the watched dir may not result in "because of renaming" flag on Win, Linux
	- may not work well with content with access permissions problems (i've made attempts to fix this, but anyways..)

*/

const EVENTS_QUEUE_CAPACITY = 100

const (
	IsError    = iota
	IsCreated  = iota
	IsModified = iota
	IsDeleted  = iota
)

var ErrorStopRequest = errors.New("monitor stop request")
var ErrorAppShutdown = errors.New("app shutdown")
var ErrorWatcherStoppedWorking = errors.New("watcher stopped working")
var ErrorUnknownEvent = errors.New("strange event type from internal watcher")

type WatcherEvent struct {
	FullFileName string
	Type         int

	StartupEvent       bool
	ItIsRenameActually bool // can be false when should be actually true! don't rely on its value
	Error              error

	WatchedDir string
	Watcher    *DirsWatcher
}

func (we *WatcherEvent) IsError() bool    { return we.Type == IsError }
func (we *WatcherEvent) IsCreated() bool  { return we.Type == IsCreated }
func (we *WatcherEvent) IsDeleted() bool  { return we.Type == IsDeleted }
func (we *WatcherEvent) IsModified() bool { return we.Type == IsModified }

type DirsWatcher struct {
	Dirs                            []string
	StopChan                        chan struct{}
	MaxDelayForModificationEventsMs int64
	Events                          chan *WatcherEvent
}

func (dw *DirsWatcher) Stop() {
	//	// decided to not close, probably it is a bad choice...
	//	dw.StopChan <- struct{}{}
	close(dw.StopChan)
}

func WatchDir(dir string) *DirsWatcher { return WatchDirs([]string{dir}, nil) }
func WatchDirs(dirs []string, stopChan chan struct{}) *DirsWatcher {
	if dirs == nil {
		return nil
	}
	ch := make(chan *WatcherEvent, EVENTS_QUEUE_CAPACITY)

	if stopChan == nil {
		stopChan = make(chan struct{}, 2)
	}
	res := &DirsWatcher{Dirs: dirs, StopChan: stopChan, Events: ch, MaxDelayForModificationEventsMs: 500}

	for _, v := range dirs {
		files, _ := ioutil.ReadDir(v)
		for _, f := range files {
			filename := getFullName(v, f.Name())
			e := &WatcherEvent{Watcher: res, WatchedDir: v, FullFileName: filename, Type: IsCreated, StartupEvent: true}
			ch <- e
		}
	}

	for _, v := range dirs {

		go func(dir string) {

			res.MaxDelayForModificationEventsMs = 500
			fileEventQueues := makeQueues(dir, res.MaxDelayForModificationEventsMs, ch)

			watcher, we := fsnotify.NewWatcher()

			done := make(chan bool)

			// Process events
			go func() {
				for {
					select {
					case ev := <-watcher.Event:
						filename := getFullName(dir, ev.Name)
						e := &WatcherEvent{Watcher: res, WatchedDir: dir, FullFileName: filename}

						if ev.IsModify() {
							e.Type = IsModified
						} else if ev.IsCreate() {
							e.Type = IsCreated
						} else if ev.IsAttrib() {
							e.Type = IsModified
						} else if ev.IsRename() {
							e.ItIsRenameActually = true
							if e.FileExists() {
								e.Type = IsCreated
							} else {
								e.Type = IsDeleted
							}
						} else if ev.IsDelete() {
							e.Type = IsDeleted
						} else {
							e.Error = ErrorUnknownEvent
						}

						fileEventQueues.Process(e)
						//ch <- e
					case err := <-watcher.Error:
						e := &WatcherEvent{Watcher: res, WatchedDir: dir, Error: err}
						ch <- e
					case <-stopChan:
						e := &WatcherEvent{Watcher: res, WatchedDir: dir, Error: ErrorStopRequest}
						ch <- e
						closeInternalWatcher(watcher, fileEventQueues)
						return
					case <-StopChannel:
						e := &WatcherEvent{Watcher: res, WatchedDir: dir, Error: ErrorAppShutdown}
						ch <- e
						closeInternalWatcher(watcher, fileEventQueues)
						return
					case <-done:
						e := &WatcherEvent{Watcher: res, WatchedDir: dir, Error: ErrorWatcherStoppedWorking}
						ch <- e
						closeInternalWatcher(watcher, fileEventQueues)
						return
					}
				}
			}()

			if we != nil {
				close(done)
			} else {
				watcher.Watch(dir)
				//				close(done)
				//				watcher.Close()
			}

		}(v)
	}

	return res
}

func closeInternalWatcher(w *fsnotify.Watcher, d *dirFileEventsProcessingStruct) {
	defer func() { recover() }()
	if w == nil {
		return
	}
	w.Close()
}

func FileExists(filename string) bool {
	if filename == "" {
		return false
	}
	// i know that OR is not needed here, just decided to leave isnotexist check for verbosity
	if _, err := os.Stat(filename); err != nil || os.IsNotExist(err) {
		return false
	}
	return true
}
func FileSizeAndModTime(filename string) (int64, time.Time) {
	if filename != "" {
		if s, err := os.Stat(filename); err == nil {
			return s.Size(), s.ModTime()
		}
	}
	return 0, time.Time{}
}
func (e *WatcherEvent) FileExists() bool {
	return FileExists(e.FullFileName)
}

func getFullName(dir string, name string) string {

	name = strings.Replace(name, "\\", "/", -1)
	if i := strings.LastIndex(name, "/"); i > -1 {
		name = name[i+1:]
	}

	if !strings.HasPrefix(dir, "/") && !strings.HasPrefix(dir, "\\") {
		dir += "/"
	}

	relative := dir + name
	res, e := filepath.Abs(dir + name)

	if e != nil {
		return relative
	}
	return res
}

func (e *WatcherEvent) String() string {
	if e.IsError() {
		return e.Error.Error()
	}
	res := "file `" + filepath.Base(e.FullFileName) + "` of dir `" + e.WatchedDir + "` "
	if e.IsModified() {
		res += "modified"
	} else if e.IsDeleted() {
		res += "deleted"
	} else if e.IsCreated() {
		if e.StartupEvent {
			res += "found"
		} else {
			res += "created"
		}
	}
	if e.ItIsRenameActually {
		res += " (after renaming)"
	}
	return res
}
