package mir

import (
	"encoding/json"
	"go/format"
	"testing"
)

func TestDSTFieldLayout(t *testing.T) {
	g := &generator{types: map[int]*Type{
		1: {ID: 1, Kind: "dynamic", Align: 1},
		2: {ID: 2, Kind: "u64", Size: 8, Align: 8, Sized: true},
		3: {ID: 3, Kind: "aggregate", Size: 16, Align: 8, Fields: []uint64{0, 8, 16}, VariantFieldTypes: [][]int{{2, 2, 1}}},
		4: {ID: 4, Kind: "aggregate", Size: 24, Align: 8, Fields: []uint64{0, 8}, VariantFieldTypes: [][]int{{2, 3}}},
		5: {ID: 5, Kind: "aggregate", Size: 2, Align: 2, Pack: 2, Fields: []uint64{0, 2}, VariantFieldTypes: [][]int{{6, 1}}},
		6: {ID: 6, Kind: "u16", Size: 2, Align: 2, Sized: true},
	}}
	g.line("package fixture\nimport \"unsafe\"")
	for _, tc := range []struct {
		name string
		typ  int
		path string
	}{
		{"Inner", 3, `[{"Deref":null},{"Field":[2,1]}]`},
		{"Nested", 4, `[{"Deref":null},{"Field":[1,3]},{"Field":[2,1]}]`},
		{"Packed", 5, `[{"Deref":null},{"Field":[1,1]}]`},
	} {
		pointer := tc.typ + 10
		g.types[pointer] = &Type{ID: pointer, Kind: "pointer", Size: 16, Align: 8, Sized: true, Pointee: tc.typ}
		g.f = &Function{Body: &Body{Locals: []Local{{Type: pointer}}}}
		var projection []json.RawMessage
		if err := json.Unmarshal([]byte(tc.path), &projection); err != nil {
			t.Fatal(err)
		}
		p := g.place(Place{Local: 0, Projection: projection})
		size, align, ok := g.dstSizeAlign(g.typ(tc.typ), "v0[1]")
		if !ok {
			t.Fatal("missing DST layout")
		}
		g.line("func %s(v0 [2]uintptr) (uintptr,uintptr,uintptr) { return uintptr(%s)-v0[0],%s,%s }", tc.name, p.address, size, align)
	}
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	testSIMDProgram(t, source, `package fixture
import("testing";"unsafe";"syscall")
func TestLayout(t *testing.T) {
 storage,err:=syscall.Mmap(-1,0,4096,syscall.PROT_READ|syscall.PROT_WRITE,syscall.MAP_PRIVATE|syscall.MAP_ANON)
 if err!=nil{t.Fatal(err)};defer syscall.Munmap(storage)
 data:=uintptr(unsafe.Pointer(unsafe.SliceData(storage)));vt:=data+256
 *(*uintptr)(unsafe.Pointer(vt+8))=64
 for _,tc:=range []struct{align uintptr; inner,nested [3]uintptr}{
  {8,[3]uintptr{16,80,8},[3]uintptr{24,88,8}},
  {32,[3]uintptr{32,96,32},[3]uintptr{64,128,32}},
  {64,[3]uintptr{64,128,64},[3]uintptr{128,192,64}},
 } {
  *(*uintptr)(unsafe.Pointer(vt+16))=tc.align
  for _,x:=range []struct{name string; f func([2]uintptr)(uintptr,uintptr,uintptr);want [3]uintptr}{
   {"inner",Inner,tc.inner},{"nested",Nested,tc.nested},{"packed",Packed,[3]uintptr{2,66,2}},
  } {
   a,b,d:=x.f([2]uintptr{data,vt})
   if got:=[3]uintptr{a,b,d};got!=x.want{t.Fatalf("%s align %d: %v want %v",x.name,tc.align,got,x.want)}
  }
 }
}
`)
}
