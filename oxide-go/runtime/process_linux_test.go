//go:build linux && (amd64 || arm64)

package oxide

import (
	"os"
	"testing"
	"unsafe"
)

func checkProcessArguments(t *testing.T, argc int64, argv uintptr, want []string) {
	t.Helper()
	if argc != int64(len(want)) {
		t.Fatalf("argc=%d, want %d", argc, len(want))
	}
	for i, s := range want {
		p := *(*uintptr)(unsafe.Pointer(argv + uintptr(i)*8))
		if p == 0 {
			t.Fatalf("argument %d is NULL", i)
		}
		if got := string(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(s))); got != s {
			t.Fatalf("argument %d=%q, want %q", i, got, s)
		}
		if *(*byte)(unsafe.Pointer(p + uintptr(len(s)))) != 0 {
			t.Fatalf("argument %d not terminated", i)
		}
	}
	if *(*uintptr)(unsafe.Pointer(argv + uintptr(len(want))*8)) != 0 {
		t.Fatal("argv lacks NULL sentinel")
	}
}

func TestProcessArgumentBytes(t *testing.T) {
	for _, args := range [][]string{nil, {"program", "", "空白 雪", "a\xff\xfeb"}} {
		argc, argv := makeProcessArguments(args)
		checkProcessArguments(t, argc, argv, args)
	}
}

func TestProcessArgumentStartupSnapshot(t *testing.T) {
	want := append([]string(nil), os.Args...)
	old := os.Args
	os.Args = []string{"changed by embedding main"}
	defer func() { os.Args = old }()
	argc, argv := ProcessArguments()
	checkProcessArguments(t, argc, argv, want)
	requireNoGoAllocations(t, 100, func() {
		n, p := ProcessArguments()
		if n != argc || p != argv {
			panic("process argument snapshot changed")
		}
	})
}
