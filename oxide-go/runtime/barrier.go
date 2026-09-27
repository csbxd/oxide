package oxide

// CompilerBarrier preserves the compiler memory clobber of an empty Rust asm
// block. A non-inlined call carries a memory dependency in Go's SSA; no CPU
// fence is needed because the original assembly has no instructions either.
//
//go:noinline
func CompilerBarrier() {}
