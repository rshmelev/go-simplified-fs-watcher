# go-simplified-fs-watcher

...probably the best and most powerful and easiest-to-use library for fs watching

```golang
w := fswatcher.WatchDir("./")

for {
    ev, ok := <-w.Events
	if !ok {
	 	break
	}
	println(ev.String())
}
```

why is it the best:

- it is a smart wrapper around [github.com/howeyc/fsnotify]
- __simple lifecycle__: created-modified-deleted pattern across all OSes
- super easy-to-use API
- can generate file-created events on watching start (for each file in directory)
- generates modification event when modification is finally finished, avoiding huge amount of intermidiate modified events
- generated "created" event after file was not only created but also all initial writing to it was finished, so file is finally ready to use
- anyways the intention is __to not send you the garbage__ (intermediate events)

limitations:

- updating child dir files may not result in Modify events on Linux
- file renaming may not result in "because of renaming" flag during the Created event on Linux
- dir deletion may not result in "because of renaming" flag during the Deleted event on Win
- some dir moving inside the watched dir may not result in "because of renaming" flag on Win, Linux
- some events may not work well with content with access permissions problems (i've made attempts to fix this, but anyways..)

each event can be stringified with `.String()` for debugging

Event properties:

- `IsItRenameActually` - indicates that this created/deleted event was raised because of moving (note: may sometimes not work properly)
- `StartupEvent` - means this event was generated during the watching startup, you can filter such events with this flag if you do not need them
- `FullFileName`
- `isError(), isCreated(), isModified(), isDeleted()` methods to get event type
- `DirWatcher.MaxDelayForModificationEventsMs` - too long to describe this prop, hopefully you understand this

author: rshmelev@gmail.com


