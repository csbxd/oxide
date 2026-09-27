package mir

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the generated methods themselves, including unaligned packed
// places. The destructor stubs only observe storage/panic behavior; the separate
// native Replace gate verifies the Rust destructor and unwind implementations.
func TestStaticStorage(t *testing.T) {
	types := []Type{
		{ID: 0, Name: "()", Kind: "aggregate", Sized: true, Align: 1},
		{ID: 1, Name: "u64", Kind: "u64", Sized: true, Size: 8, Align: 8},
		{ID: 2, Name: "u128", Kind: "u128", Sized: true, Size: 16, Align: 16},
		{ID: 3, Name: "bool", Kind: "bool", Sized: true, Size: 1, Align: 1},
		{ID: 4, Name: "char", Kind: "char", Sized: true, Size: 4, Align: 4},
		{ID: 5, Name: "test::Owner", Kind: "aggregate", Sized: true, Size: 64, Align: 64, NeedsDrop: true, DropSymbol: "dropOwner"},
		{ID: 6, Name: "*mut test::Owner", Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 5, Mutable: true},
		{ID: 7, Name: "test::Zero", Kind: "aggregate", Sized: true, Align: 32, NeedsDrop: true, DropSymbol: "dropZero"},
		{ID: 8, Name: "*mut test::Zero", Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 7, Mutable: true},
		{ID: 9, Name: "test::Large", Kind: "aggregate", Sized: true, Size: 256 << 10, Align: 64},
		{ID: 10, Name: "test::Text", Kind: "aggregate", Sized: true, Size: 24, Align: 8, NeedsDrop: true, DropSymbol: "dropText", Container: &Container{Kind: "string", Element: 11, DataOffset: 16, LenOffset: 0, CapacityOffset: 8, GlobalAllocator: true}},
		{ID: 11, Name: "u8", Kind: "u8", Sized: true, Size: 1, Align: 1},
		{ID: 12, Name: "str", Kind: "str", Align: 1},
		{ID: 13, Name: "*mut test::Text", Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 10, Mutable: true},
		{ID: 14, Name: "test::Vector", Kind: "aggregate", Sized: true, Size: 24, Align: 8, NeedsDrop: true, DropSymbol: "dropVector", Container: &Container{Kind: "vec", Element: 1, DataOffset: 0, LenOffset: 16, CapacityOffset: 8, GlobalAllocator: true}},
		{ID: 15, Name: "*mut test::Vector", Kind: "pointer", Sized: true, Size: 8, Align: 8, Pointee: 14, Mutable: true},
		{ID: 16, Name: "[u64]", Kind: "slice", Align: 8, Element: 1},
		{ID: 17, Name: "&[u64]", Kind: "pointer", Sized: true, Size: 16, Align: 8, Pointee: 16, PointerKind: "ref", Container: &Container{Kind: "slice_ref", Element: 1, LenOffset: 8}},
		{ID: 18, Name: "&mut [u64]", Kind: "pointer", Sized: true, Size: 16, Align: 8, Pointee: 16, PointerKind: "ref", Mutable: true, Container: &Container{Kind: "slice_ref", Element: 1, LenOffset: 8}},
		{ID: 19, Name: "[u64; 3]", Kind: "array", Sized: true, Size: 24, Align: 8, Element: 1, Length: 3},
		{ID: 20, Name: "f32", Kind: "f32", Sized: true, Size: 4, Align: 4},
		{ID: 21, Name: "test::PackedChar", Kind: "aggregate", Sized: true, Size: 5, Align: 1, Pack: 1, Members: [][]TypeMember{{{Name: "letter", Type: 4, Offset: 1, Public: true}}}},
		{ID: 22, Name: "test::Niche", Kind: "aggregate", AdtKind: "Enum", Sized: true, Size: 1, Align: 1, Tag: &Tag{Size: 1, Encoding: "niche", Start: "255", First: 0, Last: 1, Untagged: 2}, VariantNames: []string{"A", "B", "Some"}, Discriminants: []string{"0", "1", "2"}, Members: [][]TypeMember{nil, nil, {{Name: "0", Type: 11, Public: true}}}},
		{ID: 23, Name: "test::Wide", Kind: "aggregate", AdtKind: "Enum", Sized: true, Size: 16, Align: 16, NonExhaustive: true, Tag: &Tag{Size: 16, Encoding: "direct"}, VariantNames: []string{"Negative", "High", "Opaque"}, Discriminants: []string{"-7", "1208925819614629174706179", "9"}, VariantsInfo: []VariantInfo{{Inhabited: true}, {Inhabited: true}, {Inhabited: true, NonExhaustive: true}}},
		{ID: 24, Name: "test::Single", Kind: "aggregate", AdtKind: "Enum", Sized: true, Align: 1, Variant: 2, VariantNames: []string{"Dead0", "Dead1", "Live"}, Discriminants: []string{"0", "1", "2"}, VariantsInfo: []VariantInfo{{}, {}, {Inhabited: true}}},
		{ID: 25, Name: "test::PackedWide", Kind: "aggregate", Sized: true, Size: 17, Align: 1, Pack: 1, Members: [][]TypeMember{{{Name: "wide", Type: 2, Offset: 1, Public: true}}}},
		{ID: 26, Name: "dyn test::Marker", Kind: "dynamic", Align: 1},
		{ID: 27, Name: "core::mem::ManuallyDrop<dyn test::Marker>", Kind: "aggregate", Align: 1, Members: [][]TypeMember{{{Name: "value", Type: 26}}}},
		{ID: 28, Name: "test::PackedDST", Kind: "aggregate", Align: 2, Pack: 2, Members: [][]TypeMember{{{Name: "head", Type: 11, Public: true}, {Name: "tail", Type: 27, Offset: 2, Public: true}}}},
		{ID: 29, Name: "test::AlignedDST", Kind: "aggregate", Align: 8, Members: [][]TypeMember{{{Name: "head", Type: 1, Public: true}, {Name: "tail", Type: 27, Offset: 8, Public: true}}}},
	}
	p := &Program{APITypes: types, Types: types}
	for _, entry := range []struct {
		id   int
		name string
	}{{5, "Owner"}, {7, "Zero"}, {9, "Large"}, {10, "Text"}, {14, "Vector"}, {16, "Words"}, {17, "WordRef"}, {18, "WordMut"}, {19, "Array"}, {21, "PackedChar"}, {22, "Niche"}, {23, "Wide"}, {24, "Single"}, {25, "PackedWide"}, {26, "Dynamic"}, {27, "DynamicTail"}, {28, "PackedDST"}, {29, "AlignedDST"}} {
		p.PublicTypes = append(p.PublicTypes, PublicType{Name: "test::" + entry.name, Type: entry.id})
	}
	g := &generator{p: p, types: make(map[int]*Type), functions: make(map[string]*Function), names: make(map[string]string)}
	for i := range p.Types {
		g.types[p.Types[i].ID] = &p.Types[i]
	}
	for _, entry := range []struct {
		name    string
		pointer int
	}{{"dropOwner", 6}, {"dropZero", 8}, {"dropText", 13}, {"dropVector", 15}} {
		g.functions[entry.name] = &Function{Symbol: entry.name, Signature: &Signature{Params: []int{entry.pointer}, Return: 0}}
		g.names[entry.name] = entry.name
	}
	ids := g.collectAPITypes()
	g.initAPINames(ids)
	g.line("package generated\nimport(\"unsafe\";\"unicode/utf8\";oxide \"github.com/csbxd/oxide/oxide-go/runtime\")")
	for _, id := range ids {
		g.emitLayoutType(id)
		g.emitStaticStorage(id)
		g.emitStaticFields(id)
		g.emitStaticContainer(id)
	}
	g.line(`
var drops, zeroDrops uint64
var lastDrop, mutateSource uintptr
var throwOld bool
var marker = new(byte)
func testReadBits(p,n uintptr) (v oxide.U128) {copy(unsafe.Slice((*byte)(unsafe.Pointer(&v)),16),unsafe.Slice((*byte)(unsafe.Pointer(p)),n));return}
func testWriteBits(p,n uintptr,v oxide.U128) {copy(unsafe.Slice((*byte)(unsafe.Pointer(p)),n),unsafe.Slice((*byte)(unsafe.Pointer(&v)),16))}
func dropOwner(ctx *oxide.Context, p uintptr) {
 if p%%64!=0 {panic("unaligned destructor")}; lastDrop=p; drops++
 id:=testReadBits(p,8).Lo
 if id==1 && mutateSource!=0 {testWriteBits(mutateSource,8,oxide.U128{Lo:99})}
 if id==1 && throwOld {ctx.Fail(marker)}
}
func dropZero(_ *oxide.Context,_ uintptr){zeroDrops++}
func dropText(_ *oxide.Context,p uintptr){ data:=testReadBits(p+16,8).Lo; cap:=testReadBits(p+8,8).Lo; if cap!=0 {oxide.RustDealloc(uintptr(data),uintptr(cap),1)} }
func dropVector(_ *oxide.Context,p uintptr){ data:=testReadBits(p,8).Lo; cap:=testReadBits(p+8,8).Lo; if cap!=0 {oxide.RustDealloc(uintptr(data),uintptr(cap)*8,8)} }
`)
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "oxide.Value") || strings.Contains(string(source), "oxide.Type") || strings.Contains(string(source), "func(*oxide.Context") {
		t.Fatal("dynamic interop leaked into static storage")
	}
	if strings.Contains(string(source), "func New__Single__Dead") || strings.Contains(string(source), "func New__Wide__Opaque") {
		t.Fatal("constructor for inaccessible variant")
	}
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"go.mod":            []byte(fmt.Sprintf("module generated\n\ngo 1.27\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %s\n", filepath.ToSlash(module))),
		"generated.go":      source,
		"generated_test.go": []byte(staticStorageTest),
	}
	allocations, err := os.ReadFile(filepath.Join("testdata", "allocations_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	files["allocations_test.go"] = []byte(strings.Replace(string(allocations), "package fixture", "package generated", 1))
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"arm64", "amd64"} {
		args := []string{"test", "-mod=mod", "-gcflags=generated=-smallframes", "-count=1"}
		if arch != runtime.GOARCH {
			args = append(args, "-c", "-o", filepath.Join(dir, arch+".test"))
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s\nsource directory %s", arch, err, out, dir)
		}
	}
	// Compile the exact generated constructors as a same-package test overlay.
	// This can use the existing mapping-failure injector without a production
	// test hook, copied constructor implementation, or checked-in generated file.
	overlaySource := strings.Replace(string(source), "package generated", "package oxide", 1)
	overlaySource = strings.Replace(overlaySource, `oxide "github.com/csbxd/oxide/oxide-go/runtime"`, `"testing"`, 1)
	overlaySource = strings.ReplaceAll(overlaySource, "oxide.", "") + staticStorageFailureTest
	overlayFile := filepath.Join(dir, "runtime_overlay.go")
	if err := os.WriteFile(overlayFile, []byte(overlaySource), 0600); err != nil {
		t.Fatal(err)
	}
	virtual := filepath.Join(module, "runtime", "zz_static_api_generated_test.go")
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{virtual: overlayFile}})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-tags=memory.counters", "-overlay", overlayPath, "./runtime", "-run", "^TestGeneratedStaticHeapFailure$", "-count=1")
	cmd.Dir = module
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated failure overlay: %v\n%s", err, out)
	}
}

const staticStorageTest = `package generated
import("testing"; "math"; oxide "github.com/csbxd/oxide/oxide-go/runtime")
func caught(fn func()) (failed bool) {defer func(){failed=recover()!=nil}(); fn(); return}
func TestGeneratedStorage(t *testing.T) {
 ctx:=oxide.NewContext(); defer ctx.Close()
 run:=func(){
  mark:=ctx.Mark()
  u:=New__U64(ctx);u.Mut().Set(0xfedcba9876543210);if u.Ref().Get()!=0xfedcba9876543210 {t.Fatal("scalar")}
  b:=New__Bool(ctx);b.Mut().Set(true);if !b.Ref().Get(){t.Fatal("bool")}
  ch:=New__Char(ctx);ch.Mut().Set('界');if ch.Ref().Get()!='界'{t.Fatal("char")}
  packedChar:=New__PackedChar(ctx);packedChar.Mut().Field__Letter().Set(0x1f600);if packedChar.Ref().Read__Letter()!=0x1f600{t.Fatal("packed char")}
  f:=New__F32(ctx);f.Mut().Set(math.Float32frombits(0x80000000));if math.Float32bits(f.Ref().Get())!=0x80000000{t.Fatal("negative zero")}
  wide:=New__U128(ctx);want:=oxide.U128{Lo:1,Hi:99};wide.Mut().Set(want);if wide.Ref().Get()!=want || wide.Addr()%16!=0 {t.Fatal("wide")}
  packedWide:=New__PackedWide(ctx);packedWide.Mut().Field__Wide().Set(want);if packedWide.Ref().Read__Wide()!=want{t.Fatal("packed u128")}
  region:=ctx.Alloc(384,64);vtable:=ctx.Alloc(24,8);testWriteBits(vtable+8,8,oxide.U128{Lo:64});testWriteBits(vtable+16,8,oxide.U128{Lo:64})
  packedDST:=UnsafeMut__PackedDST(region,vtable)
  if packedDST.Size()!=66 || packedDST.Align()!=2 || packedDST.Field__Tail().Addr()!=region+2{t.Fatal("packed dynamic raw layout")}
  if !caught(func(){packedDST.Ref().Field__Tail()}) || !caught(func(){packedDST.Field__Tail().Ref()}){t.Fatal("packed dynamic field escaped as aligned Rust reference")}
  alignedTail:=UnsafeRef__PackedDST(region+62,vtable).Field__Tail()
  if alignedTail.Addr()!=region+64 || alignedTail.Align()!=64{t.Fatal("valid aligned field in packed dynamic object")}
  alignedDST:=UnsafeRef__AlignedDST(region+128,vtable)
  if alignedDST.Size()!=128 || alignedDST.Align()!=64 || alignedDST.Field__Tail().Addr()!=region+192{t.Fatal("ordinary dynamic tail borrow")}
  na:=New__Niche__A(ctx);nb:=New__Niche__B(ctx);ns:=New__Niche__Some(ctx,7)
  if na.Ref().Variant()!=Variant__Niche__A || nb.Ref().Variant()!=Variant__Niche__B || ns.Ref().Variant()!=Variant__Niche__Some || ns.Ref().Field__Some__0().Get()!=7 || testReadBits(na.Addr(),1).Lo!=255 || testReadBits(nb.Addr(),1).Lo!=0{t.Fatal("wrapped niche")}
  neg:=New__Wide__Negative(ctx);high:=New__Wide__High(ctx)
  if neg.Ref().Variant()!=Variant__Wide__Negative || high.Ref().Variant()!=Variant__Wide__High || testReadBits(neg.Addr(),16)!=oxide.U128(oxide.I128From64(-7)) || testReadBits(high.Addr(),16)!=(oxide.U128{Lo:3,Hi:1<<16}){t.Fatal("wide tag")}
  if New__Single__Live(ctx).Ref().Variant()!=Variant__Single__Live{t.Fatal("single variant index")}
  for _,panics:=range []bool{false,true}{for _,packed:=range []bool{false,true}{
   drops=0;throwOld=panics
   a:=New__Owner(ctx);rhs:=New__Owner(ctx);if a.Addr()%64!=0{t.Fatal("alignment")}
   dest:=a.Mut();if packed{dest=UnsafeMut__Owner(ctx.Alloc(128,64)+1)}
   testWriteBits(dest.Addr(),8,oxide.U128{Lo:1});testWriteBits(rhs.Addr(),8,oxide.U128{Lo:2})
   mutateSource=rhs.Addr();before:=ctx.Mark()
   if caught(func(){dest.Replace(ctx,rhs)})!=panics{t.Fatal("panic propagation")}
   if ctx.Mark()!=before || ctx.Failed() || testReadBits(dest.Addr(),8).Lo!=2 || drops!=1{t.Fatal("replacement")}
   if (!packed && lastDrop!=dest.Addr()) || lastDrop%64!=0{t.Fatal("drop address")}
   current:=dest.Move(ctx);current.Drop(ctx);if drops!=2{t.Fatal("consumed rhs")}
   mutateSource=0;throwOld=false
  }}
  zeroDrops=0;z:=UnsafeValue__Zero(32);z.Replace(ctx,UnsafeValue__Zero(32));z.Drop(ctx);if zeroDrops!=2{t.Fatal("same-address ZST")}
  drops=0;self:=New__Owner(ctx);self.Replace(ctx,self);if drops!=0{t.Fatal("self alias")}
  large:=New__Large(ctx);testWriteBits(large.Addr()+262136,8,oxide.U128{Lo:37});moved:=large.Mut().Move(ctx);if testReadBits(moved.Addr()+262136,8).Lo!=37{t.Fatal("large move")}
  h:=Alloc__Owner();testWriteBits(h.Addr(),8,oxide.U128{Lo:2});h.Close(ctx);if h.Addr()!=0{t.Fatal("storage free")}
  h=Alloc__Owner();testWriteBits(h.Addr(),8,oxide.U128{Lo:1});address:=h.Addr();throwOld=true
  if !caught(func(){h.Close(ctx)}) || h.Addr()!=0 || lastDrop!=address || ctx.Failed(){t.Fatal("storage close panic")};throwOld=false
  text:=String__Text(ctx,"hello界");if text.Ref().String()!="hello界" || text.Ref().Borrow().String()!=text.Ref().String(){t.Fatal("text")};text.Drop(ctx)
  empty:=Borrow__Str(oxide.Span{});if empty.Len()!=0 || empty.Addr()==0{t.Fatal("empty borrow")}
  vec:=Vec__Vector(ctx,3);for i:=uintptr(0);i<3;i++{x:=New__U64(ctx);x.Mut().Set(uint64(i+10));vec.Mut().InitAt(i,x)};vec.Mut().SetLen(3)
  shared:=New__WordRef(ctx);shared.Mut().SetRef(vec.Ref().Slice());if shared.Mut().Deref().Index(2).Get()!=12{t.Fatal("shared fat reference")}
  mutable:=New__WordMut(ctx);mutable.Mut().SetRef(vec.Mut().Slice());mutable.Mut().Deref().Index(1).Set(22);if vec.Ref().Index(1).Get()!=22{t.Fatal("mutable fat reference")};vec.Drop(ctx)
  ctx.Restore(mark)
 }
 run();requireNoGoAllocations(t,100,run)
 if !caught(func(){UnsafeMut__U64(ctx.Alloc(16,8)+1).Ref()}){t.Fatal("unaligned reference accepted")}
 beforeInvalid:=ctx.Mark()
 if !caught(func(){String__Text(ctx,string([]byte{255}))}){t.Fatal("UTF8 accepted")}
 if ctx.Mark()!=beforeInvalid{t.Fatal("UTF8 consumed automatic storage")}
 if !caught(func(){New__Char(ctx).Mut().Set(0xd800)}){t.Fatal("surrogate accepted")}
}
`

const staticStorageFailureTest = `
func TestGeneratedStaticHeapFailure(t *testing.T) {
 ctx:=NewContext();defer ctx.Close()
 baseline:=HeapStats().LiveAllocations
 s:=Alloc__Text();s.Init(String__Text(ctx,"owned payload"))
 if HeapStats().LiveAllocations!=baseline+2{t.Fatal("outer slot and string payload not independently owned")}
 s.Drop(ctx)
 if HeapStats().LiveAllocations!=baseline+1{t.Fatal("Drop freed slot or retained payload")}
 s.Free()
 if HeapStats().LiveAllocations!=baseline || s.Addr()!=0{t.Fatal("Free retained storage")}
 drops=0;raw:=Alloc__Owner();raw.Free()
 if drops!=0 || HeapStats().LiveAllocations!=baseline{t.Fatal("Free ran Drop")}
 mark:=ctx.Mark()
 rustHeap.mu.Lock();rustHeap.pages.FailNextMapping();rustHeap.mu.Unlock()
 before:=HeapStats()
 failed:=false
 func(){defer func(){failed=recover()!=nil}();Vec__Vector(ctx,(2<<20)/8+1)}()
 if !failed || ctx.Mark()!=mark || HeapStats()!=before{t.Fatal("mapping failure changed heap/frame")}
 vector:=Vec__Vector(ctx,3);vector.Drop(ctx)
 if HeapStats().LiveAllocations!=baseline{t.Fatal("allocator did not recover after mapping failure")}
}
`
