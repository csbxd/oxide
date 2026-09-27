// Copyright 2017 The Memory Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build memory.counters
// +build memory.counters

package memory

import "unsafe"

const counters = true

type mappingFault struct{ failNext bool }

func (f *mappingFault) failMapping() bool {
	fail := f.failNext
	f.failNext = false
	return fail
}

// FailNextMapping injects one allocation failure before the next OS mapping.
// Like Allocator itself, this test-only operation requires external locking.
func (a *Allocator) FailNextMapping() { a.failNext = true }

// Mapping describes the registered mapping containing an owned allocation.
// The caller must keep p allocated and externally synchronize the allocator.
func (a *Allocator) Mapping(p uintptr) (uintptr, int) {
	base := p &^ uintptr(pageMask)
	return base, (*page)(unsafe.Pointer(base)).size
}
