//go:build memory.counters && linux && (amd64 || arm64)

package oxide

func init() {
	contextChaosLiveHeap = func() int { return HeapStats().LiveAllocations }
}
