package util

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/satori/go.uuid"
)

// UID generates a unique id
func UID() string {
	return uuid.NewV4().String()
}

// Wait is a sync.WaitGroup.Wait() implementation that supports timeouts
func Wait(wg *sync.WaitGroup, timeout time.Duration) bool {
	wgDone := make(chan bool)
	defer close(wgDone)
	go func() {
		wg.Wait()
		wgDone <- true
	}()

	select {
	case <-wgDone:
		return true
	case <-time.After(timeout):
		return false
	}
}

func ConvertStructsToMap(i interface{}) (map[string]interface{}, error) {
	ds, err := json.Marshal(i)
	if err != nil {
		return nil, err
	}
	var mp map[string]interface{}
	err = json.Unmarshal(ds, &mp)
	if err != nil {
		return nil, err
	}
	return mp, nil
}

func MustConvertStructsToMap(i interface{}) map[string]interface{} {
	if result, err := ConvertStructsToMap(i); err != nil {
		panic(err)
	} else {
		return result
	}
}

func Truncate(val interface{}, maxLen int) string {
	s := fmt.Sprintf("%v", val)
	if len(s) <= maxLen {
		return s
	}
	affix := fmt.Sprintf("<truncated orig_len: %d>", len(s))
	return s[:len(s)-len(affix)] + affix
}

// SyncMapLen is simply a sync.Map with options to retrieve the length of it.
type SyncMapLen struct {
	mp        sync.Map
	cachedLen *int32
}

func (e *SyncMapLen) Len() int {
	if e.cachedLen == nil {
		var count int32
		e.mp.Range(func(k, v interface{}) bool {
			count++
			return true
		})
		atomic.StoreInt32(e.cachedLen, count)
		return int(count)
	}
	return int(atomic.LoadInt32(e.cachedLen))
}

func (e *SyncMapLen) LoadOrStore(key interface{}, value interface{}) (actual interface{}, loaded bool) {
	actual, loaded = e.mp.LoadOrStore(key, value)
	if !loaded && e.cachedLen != nil {
		atomic.AddInt32(e.cachedLen, 1)
	}
	return actual, loaded
}

func (e *SyncMapLen) Load(key interface{}) (value interface{}, ok bool) {
	return e.mp.Load(key)
}

func (e *SyncMapLen) Store(key, value interface{}) {
	e.mp.Store(key, value)
	e.cachedLen = nil // We are not sure if an entry was replaced or added
}

func (e *SyncMapLen) Delete(id string) {
	e.mp.Delete(id)
	e.cachedLen = nil // We are not sure if an entry was removed
}

func (e *SyncMapLen) Range(f func(key interface{}, value interface{}) bool) {
	e.mp.Range(f)
}
