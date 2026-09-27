//go:build linux && (amd64 || arm64)

package oxide

import (
	"sync"
	"syscall"
	"unsafe"
)

// DestructorCallback is generated from the Rust function pointer's exact ABI.
// Arguments are the current Context, function descriptor and destructor value.
type DestructorCallback func(*Context, uintptr, uintptr)

type pthreadKey struct {
	generation uint64
	active     bool
	destructor uintptr
	invoke     DestructorCallback
}
type pthreadValue struct {
	generation uint64
	value      uintptr
}
type threadDestructor struct {
	destructor uintptr
	value      uintptr
	dso        uintptr
	invoke     DestructorCallback
}

const pthreadKeysMax = 1024
const pthreadDestructorIterations = 4

var pthreadKeys struct {
	sync.Mutex
	keys [pthreadKeysMax]pthreadKey
}

func LibcPthreadKeyCreate(c *Context, out, destructor uintptr, invoke DestructorCallback) int32 {
	if destructor != 0 && invoke == nil {
		panic("oxide: pthread destructor requires a translated callback adapter")
	}
	pthreadKeys.Lock()
	defer pthreadKeys.Unlock()
	for key := range pthreadKeys.keys {
		slot := &pthreadKeys.keys[key]
		if slot.active {
			continue
		}
		slot.generation++
		slot.active = true
		slot.destructor, slot.invoke = destructor, invoke
		*(*uint32)(unsafe.Pointer(out)) = uint32(key)
		return 0
	}
	return int32(syscall.EAGAIN)
}

func LibcPthreadKeyDelete(c *Context, key uint32) int32 {
	pthreadKeys.Lock()
	defer pthreadKeys.Unlock()
	if key >= pthreadKeysMax || !pthreadKeys.keys[key].active {
		return int32(syscall.EINVAL)
	}
	slot := &pthreadKeys.keys[key]
	slot.active, slot.destructor, slot.invoke = false, 0, nil
	return 0
}

func currentPthreadKey(key uint32) pthreadKey {
	pthreadKeys.Lock()
	defer pthreadKeys.Unlock()
	if key >= pthreadKeysMax {
		return pthreadKey{}
	}
	return pthreadKeys.keys[key]
}

func LibcPthreadSetspecific(c *Context, key uint32, value uintptr) int32 {
	slot := currentPthreadKey(key)
	if !slot.active {
		return int32(syscall.EINVAL)
	}
	if value == 0 {
		delete(c.pthreadValues, key)
		return 0
	}
	if c.pthreadValues == nil {
		c.pthreadValues = make(map[uint32]pthreadValue)
	}
	c.pthreadValues[key] = pthreadValue{generation: slot.generation, value: value}
	return 0
}

func LibcPthreadGetspecific(c *Context, key uint32) uintptr {
	slot := currentPthreadKey(key)
	value := c.pthreadValues[key]
	if !slot.active || slot.generation != value.generation {
		return 0
	}
	return value.value
}

func LibcCxaThreadAtExit(c *Context, destructor, value, dso uintptr, invoke DestructorCallback) int32 {
	if destructor == 0 || invoke == nil {
		panic("oxide: C++ TLS destructor requires a translated callback adapter")
	}
	// Translated Go modules have static lifetime. Keep the module cookie with
	// each registration; dynamic unloading is not part of this runtime's ABI.
	c.cxaDestructors = append(c.cxaDestructors, threadDestructor{destructor, value, dso, invoke})
	return 0
}

func (c *Context) closeThreadDestructors() {
	c.closeCxaDestructors()
	c.closePthreadDestructors()
}

func (c *Context) closeCxaDestructors() {
	// Match Linux libc: C++ TLS destructors precede pthread key destructors.
	// A destructor can register another destructor, which must run next.
	for len(c.cxaDestructors) != 0 {
		i := len(c.cxaDestructors) - 1
		d := c.cxaDestructors[i]
		c.cxaDestructors[i] = threadDestructor{}
		c.cxaDestructors = c.cxaDestructors[:i]
		d.invoke(c, d.destructor, d.value)
	}
}

func (c *Context) closePthreadDestructors() {
	for range pthreadDestructorIterations {
		// Snapshot the keys so callbacks that replace values or create keys
		// cannot make one key run more than once in a destructor iteration.
		var keys [pthreadKeysMax]uint32
		n := 0
		for key := range c.pthreadValues {
			keys[n] = key
			n++
		}
		invoked := false
		for _, key := range keys[:n] {
			value := c.pthreadValues[key]
			delete(c.pthreadValues, key)
			slot := currentPthreadKey(key)
			if value.value == 0 || !slot.active || slot.generation != value.generation || slot.destructor == 0 {
				continue
			}
			invoked = true
			slot.invoke(c, slot.destructor, value.value)
		}
		if !invoked {
			break
		}
	}
}
