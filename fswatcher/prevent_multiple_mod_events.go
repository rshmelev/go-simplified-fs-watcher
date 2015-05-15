package advfswatcher

import (
	"os"
	"strings"
	"sync"
	"time"
)

type dirFileEventsProcessingStruct struct {
	sync.Mutex
	files              map[string]*fileEventsProcessingStruct
	resultingChan      chan *WatcherEvent
	modTrackingDelayMs int64
}

func makeQueues(dir string, MaxDelayForModificationEventsMs int64, reschan chan *WatcherEvent) *dirFileEventsProcessingStruct {
	r := &dirFileEventsProcessingStruct{
		files:              make(map[string]*fileEventsProcessingStruct),
		resultingChan:      reschan,
		modTrackingDelayMs: MaxDelayForModificationEventsMs,
	}
	if r.modTrackingDelayMs == 0 {
		r.modTrackingDelayMs = 1000
	}
	return r
}

func (d *dirFileEventsProcessingStruct) Remove(filename string) {
	d.Lock()
	delete(d.files, filename)
	d.Unlock()
}
func (d *dirFileEventsProcessingStruct) Process(ev *WatcherEvent) {
	if d.modTrackingDelayMs <= 0 {
		d.resultingChan <- ev
		return
	}

	d.Lock()
	f, ok := d.files[ev.FullFileName]
	if !ok {
		//fmt.Println("watching: " + ev.FullFileName)
		f = &fileEventsProcessingStruct{
			fileName:      ev.FullFileName,
			papa:          d,
			lastEventTime: makeTimestampMs(),
		}
		go f.AutoRemoveSomeday()
		d.files[ev.FullFileName] = f
	}
	d.Unlock()

	f.Process(ev)
}

type fileEventsProcessingStruct struct {
	sync.Mutex
	fileName                string
	lastEventTime           int64
	lastModifiedEventTime   int64
	queuedModificationEvent *WatcherEvent
	papa                    *dirFileEventsProcessingStruct
}

func (p *fileEventsProcessingStruct) AutoRemoveSomeday() {
	memory := p.lastEventTime
	lastmod := memory

	fileSize, modTime := FileSizeAndModTime(p.fileName)

	var divider int64 = 4
	changed := false

	for {
		time.Sleep(time.Millisecond * time.Duration((p.papa.modTrackingDelayMs / divider)))

		now := makeTimestampMs()
		if memory != p.lastEventTime {
			lastmod = now
		}

		isbusy := IsFileBusy(p.fileName)

		if now-lastmod > p.papa.modTrackingDelayMs*2 && !changed && !isbusy {
			p.papa.Remove(p.fileName)
			break
		}

		p.Lock()
		if p.queuedModificationEvent != nil {
			fileSize2, modTime2 := FileSizeAndModTime(p.fileName)
			changed = fileSize != fileSize2 || modTime.UnixNano() != modTime2.UnixNano()
			fileSize = fileSize2
			modTime = modTime2
			//fmt.Println(fileSize2, modTime2, changed)
		}

		if p.queuedModificationEvent != nil &&
			now-p.lastModifiedEventTime > p.papa.modTrackingDelayMs &&
			!changed &&
			!isbusy {

			p.papa.resultingChan <- p.queuedModificationEvent
			p.queuedModificationEvent = nil
		}
		p.Unlock()
	}
}

func IsFileBusy(filename string) bool {
	sf, err := os.Stat(filename)
	if err != nil || !sf.Mode().IsRegular() {
		return false
	}

	//f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	f, err := os.OpenFile(filename, os.O_RDONLY, 0600)

	if err != nil {
		// dirty hacks everywhere
		if os.ErrPermission == err || strings.Contains(err.Error(), "permission denied") {
			return false
		}
		//fmt.Println(err)
		return true
	}

	f.Close()
	return false
}

func (p *fileEventsProcessingStruct) Process(ev *WatcherEvent) {
	p.lastEventTime = makeTimestampMs()

	p.Lock()

	if ev.IsModified() || ev.IsCreated() {
		p.lastModifiedEventTime = p.lastEventTime
		if p.queuedModificationEvent == nil || !p.queuedModificationEvent.IsCreated() {
			//fmt.Println("...setting ", ev)
			p.queuedModificationEvent = ev
		} else if ev.IsCreated() {
			// unbelievable! one create event came after another!
			// let's publish previous created event and delay the newest
			p.papa.resultingChan <- p.queuedModificationEvent
			p.queuedModificationEvent = ev
		}
	} else {
		if p.queuedModificationEvent != nil {
			p.papa.resultingChan <- p.queuedModificationEvent
			p.queuedModificationEvent = nil
		}
		p.papa.resultingChan <- ev
	}

	p.Unlock()

}

func makeTimestampMs() int64 {
	return int64(time.Nanosecond) * time.Now().UnixNano() / int64(time.Millisecond)
	//same but not cool: return time.Now().UnixNano() / int64(time.Millisecond)
}
