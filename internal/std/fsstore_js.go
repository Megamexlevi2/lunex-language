//go:build js

package std

import (
	"sync"
	"syscall/js"
)

const fsIndexedDBName = "lunex_fs"
const fsIndexedDBStore = "files"
const fsIndexedDBVersion = 1

type fsIndexedDBBackend struct {
	mu      sync.Mutex
	db      js.Value
	ready   bool
	usable  bool
	checked bool
}

var fsBackend = &fsIndexedDBBackend{}

func fsHasIndexedDB() bool {
	global := js.Global()
	idb := global.Get("indexedDB")
	return !idb.IsUndefined() && !idb.IsNull()
}

func (b *fsIndexedDBBackend) init(onReady func()) {
	b.mu.Lock()
	if b.checked {
		ready := b.ready
		b.mu.Unlock()
		if ready && onReady != nil {
			onReady()
		}
		return
	}
	b.checked = true
	b.mu.Unlock()

	if !fsHasIndexedDB() {
		b.mu.Lock()
		b.usable = false
		b.mu.Unlock()
		if onReady != nil {
			onReady()
		}
		return
	}

	global := js.Global()
	idb := global.Get("indexedDB")
	openReq := idb.Call("open", fsIndexedDBName, fsIndexedDBVersion)

	openReq.Set("onupgradeneeded", js.FuncOf(func(this js.Value, args []js.Value) any {
		ev := args[0]
		database := ev.Get("target").Get("result")
		objectStoreNames := database.Get("objectStoreNames")
		hasStore := false
		for i := 0; i < objectStoreNames.Get("length").Int(); i++ {
			if objectStoreNames.Call("item", i).String() == fsIndexedDBStore {
				hasStore = true
				break
			}
		}
		if !hasStore {
			database.Call("createObjectStore", fsIndexedDBStore)
		}
		return nil
	}))

	openReq.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) any {
		database := args[0].Get("target").Get("result")
		b.mu.Lock()
		b.db = database
		b.ready = true
		b.usable = true
		b.mu.Unlock()
		if onReady != nil {
			onReady()
		}
		return nil
	}))

	openReq.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		b.mu.Lock()
		b.usable = false
		b.mu.Unlock()
		if onReady != nil {
			onReady()
		}
		return nil
	}))
}

func (b *fsIndexedDBBackend) isUsable() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.usable && b.ready
}

func (b *fsIndexedDBBackend) transaction(mode string) (js.Value, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.usable || !b.ready || b.db.IsUndefined() || b.db.IsNull() {
		return js.Value{}, false
	}
	tx := b.db.Call("transaction", []any{fsIndexedDBStore}, mode)
	return tx.Call("objectStore", fsIndexedDBStore), true
}

func (b *fsIndexedDBBackend) putAsync(key string, value []byte) {
	store, ok := b.transaction("readwrite")
	if !ok {
		return
	}
	arr := js.Global().Get("Uint8Array").New(len(value))
	js.CopyBytesToJS(arr, value)
	store.Call("put", arr, key)
}

func (b *fsIndexedDBBackend) deleteAsync(key string) {
	store, ok := b.transaction("readwrite")
	if !ok {
		return
	}
	store.Call("delete", key)
}

func (b *fsIndexedDBBackend) deletePrefixAsync(prefix string) {
	store, ok := b.transaction("readwrite")
	if !ok {
		return
	}
	cursorReq := store.Call("openCursor")
	cursorReq.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) any {
		cursor := args[0].Get("target").Get("result")
		if cursor.IsNull() || cursor.IsUndefined() {
			return nil
		}
		key := cursor.Get("key").String()
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			cursor.Call("delete")
		}
		cursor.Call("continue")
		return nil
	}))
}

func (b *fsIndexedDBBackend) loadAll(onEntry func(key string, value []byte), onDone func()) {
	store, ok := b.transaction("readonly")
	if !ok {
		if onDone != nil {
			onDone()
		}
		return
	}
	cursorReq := store.Call("openCursor")
	cursorReq.Set("onsuccess", js.FuncOf(func(this js.Value, args []js.Value) any {
		cursor := args[0].Get("target").Get("result")
		if cursor.IsNull() || cursor.IsUndefined() {
			if onDone != nil {
				onDone()
			}
			return nil
		}
		key := cursor.Get("key").String()
		jsVal := cursor.Get("value")
		length := jsVal.Get("length").Int()
		buf := make([]byte, length)
		js.CopyBytesToGo(buf, jsVal)
		if onEntry != nil {
			onEntry(key, buf)
		}
		cursor.Call("continue")
		return nil
	}))
	cursorReq.Set("onerror", js.FuncOf(func(this js.Value, args []js.Value) any {
		if onDone != nil {
			onDone()
		}
		return nil
	}))
}
