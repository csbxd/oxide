//go:build linux && (amd64 || arm64)

package oxide

import (
	"reflect"
	"syscall"
	"testing"
	"unsafe"
)

func TestThreadDestructorsOrderAndRegistration(t *testing.T) {
	c := NewContext()
	keyStorage := c.Alloc(4, 4)
	var order []uintptr
	var callback DestructorCallback
	callback = func(ctx *Context, descriptor, value uintptr) {
		if ctx != c || descriptor != 1 {
			t.Error("destructor called with incorrect Context or function")
		}
		order = append(order, value)
		if value == 2 {
			LibcCxaThreadAtExit(ctx, 1, 3, 17, callback)
		}
		// Context memory and errno must stay alive throughout destructors.
		*(*int32)(unsafe.Pointer(LibcErrnoLocation(ctx))) = int32(value)
	}
	if LibcPthreadKeyCreate(c, keyStorage, 1, callback) != 0 {
		t.Fatal("key_create failed")
	}
	key := *(*uint32)(unsafe.Pointer(keyStorage))
	defer LibcPthreadKeyDelete(c, key)
	LibcPthreadSetspecific(c, key, 4)
	LibcCxaThreadAtExit(c, 1, 1, 17, callback)
	LibcCxaThreadAtExit(c, 1, 2, 17, callback)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []uintptr{2, 3, 1, 4}) {
		t.Fatalf("destructor order: %v", order)
	}
	if err := c.Close(); err != nil || len(order) != 4 {
		t.Fatal("closing twice repeated a destructor")
	}
}

func TestPthreadDestructorIterations(t *testing.T) {
	c := NewContext()
	keyStorage := c.Alloc(4, 4)
	var key uint32
	calls := 0
	callback := func(ctx *Context, _, value uintptr) {
		calls++
		if LibcPthreadGetspecific(ctx, key) != 0 {
			t.Error("key value was not cleared before destructor")
		}
		LibcPthreadSetspecific(ctx, key, value+1)
	}
	if LibcPthreadKeyCreate(c, keyStorage, 1, callback) != 0 {
		t.Fatal("key_create failed")
	}
	key = *(*uint32)(unsafe.Pointer(keyStorage))
	defer LibcPthreadKeyDelete(c, key)
	LibcPthreadSetspecific(c, key, 1)
	c.Close()
	if calls != 4 {
		t.Fatalf("destructor ran %d times, want PTHREAD_DESTRUCTOR_ITERATIONS=4", calls)
	}
}

func TestPthreadKeyReuseAndContextIsolation(t *testing.T) {
	a, b := NewContext(), NewContext()
	storage := a.Alloc(4, 4)
	calls := 0
	callback := func(*Context, uintptr, uintptr) { calls++ }
	LibcPthreadKeyCreate(a, storage, 1, callback)
	key := *(*uint32)(unsafe.Pointer(storage))
	LibcPthreadSetspecific(a, key, 11)
	LibcPthreadSetspecific(b, key, 22)
	if LibcPthreadGetspecific(a, key) != 11 || LibcPthreadGetspecific(b, key) != 22 {
		t.Fatal("key storage is shared across Rust threads")
	}
	if LibcPthreadKeyDelete(a, key) != 0 || calls != 0 {
		t.Fatal("key_delete invoked a destructor")
	}
	if LibcPthreadSetspecific(a, key, 1) != int32(syscall.EINVAL) {
		t.Fatal("setspecific accepted a deleted key")
	}
	LibcPthreadKeyCreate(a, storage, 1, callback)
	newKey := *(*uint32)(unsafe.Pointer(storage))
	defer LibcPthreadKeyDelete(a, newKey)
	if key != newKey {
		t.Fatal("test did not exercise key slot reuse")
	}
	if LibcPthreadGetspecific(a, newKey) != 0 || LibcPthreadGetspecific(b, newKey) != 0 {
		t.Fatal("new key inherited an old key's value")
	}
	LibcPthreadSetspecific(b, newKey, 33)
	a.Close()
	if calls != 0 {
		t.Fatal("old key value ran the replacement destructor")
	}
	b.Close()
	if calls != 1 {
		t.Fatalf("replacement key destructor ran %d times", calls)
	}
}
