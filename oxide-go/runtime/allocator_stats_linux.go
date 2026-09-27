//go:build memory.counters && linux && (amd64 || arm64)

package oxide

// HeapSnapshot describes only the owned Rust/C heap. Context, TLS and static
// mappings are separate. Cached mappings can also contain live allocations;
// their bytes are not a measure of unused space while user objects are live.
type HeapSnapshot struct {
	LiveAllocations    int
	BackingAllocations int
	CachedAllocations  int
	Mappings           int
	MappedBytes        int
	CachedCapacity     int
	CachedMappings     int
	CachedMappedBytes  int
}

// HeapStats returns a locked snapshot. Leak checks must quiesce their workers
// first: an in-flight realloc can temporarily own both old and new storage.
func HeapStats() HeapSnapshot {
	rustHeap.mu.Lock()
	defer rustHeap.mu.Unlock()
	s := HeapSnapshot{
		BackingAllocations: rustHeap.pages.Allocs,
		Mappings:           rustHeap.pages.Mmaps,
		MappedBytes:        rustHeap.pages.Bytes,
	}
	var seen [heapCacheMaxLog + 1]uintptr
	for log, p := range rustHeap.spare {
		if p == 0 {
			continue
		}
		s.CachedAllocations++
		s.CachedCapacity += 1 << log
		base, size := rustHeap.pages.Mapping(p)
		found := false
		for _, old := range seen[:s.CachedMappings] {
			found = found || old == base
		}
		if !found {
			seen[s.CachedMappings] = base
			s.CachedMappings++
			s.CachedMappedBytes += size
		}
	}
	s.LiveAllocations = s.BackingAllocations - s.CachedAllocations
	return s
}
