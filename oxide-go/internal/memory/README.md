This package is a local copy of `modernc.org/memory v1.11.0` from
[the upstream memory project](https://gitlab.com/cznic/memory). Its source,
tests, copyright notices and BSD licenses are retained here.

The allocator algorithm is unchanged. The page registry used by `Close` is
an intrusive list in the mmap-backed page headers instead of a Go map. This
prevents map growth from allocating Go heap objects during Rust allocation.
The header grows from 32 to 48 bytes on 64-bit targets and remains aligned to
16 bytes. Package import comments were removed for this internal import path.
The upstream tests' registry-empty checks use the list head instead of a map
length; their allocation workloads and assertions are otherwise unchanged.

`memory.counters` adds mapping inspection and a one-shot mapping-failure hook
for leak tests. Failed allocation requests do not increment the live-allocation
counter. These diagnostics have no state or branch cost in normal builds.

Only Oxide's owned Rust/C allocations use this copy. `modernc.org/libc` still
uses its upstream allocator; this package does not replace dependencies in a
consumer's module graph. Additional registry tests check arbitrary page
removal, complete closure, alignment and exact Go allocation counts.
