package mir

import (
	"encoding/json"
	"go/format"
	"testing"
)

func TestSIMDValueOperands(t *testing.T) {
	g := &generator{types: map[int]*Type{
		1: {ID: 1, Kind: "u32", Size: 4, Align: 4, Sized: true},
		2: {ID: 2, Kind: "u64", Size: 8, Align: 8, Sized: true},
		3: {ID: 3, Kind: "array", Size: 16, Align: 4, Sized: true, Element: 1, Length: 4},
		4: {ID: 4, Kind: "array", Size: 32, Align: 8, Sized: true, Element: 2, Length: 4},
		5: {ID: 5, Kind: "array", Size: 16, Align: 8, Sized: true, Element: 2, Length: 2},
	}}
	g.line("package fixture\nimport \"unsafe\"")
	for _, id := range []int{3, 4, 5} {
		g.typeDecl(g.types[id])
	}
	copy1 := json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`)
	constant := func(typ, width int, values ...uint64) json.RawMessage {
		var data []int
		for _, value := range values {
			for i := 0; i < width; i++ {
				data = append(data, int(value>>(i*8)&255))
			}
		}
		b, err := json.Marshal(map[string]any{"Constant": map[string]any{"const_": map[string]any{"ty": typ, "kind": map[string]any{"Allocated": map[string]any{"bytes": data}}}}})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	g.f = &Function{Body: &Body{Locals: []Local{{Type: 5}, {Type: 5}}}}
	g.line("func Shuffle(x [2]uint64) [2]uint64 { v1:=*(*T5)(unsafe.Pointer(&x))")
	g.simdIntrinsic("simd_shuffle", []json.RawMessage{copy1, constant(5, 8, 11, 22), constant(0, 4, 2, 0)}, Place{Local: 1})
	g.line("return *(*[2]uint64)(unsafe.Pointer(&v1)) }")
	g.line("func Insert(x [2]uint64) [2]uint64 { v1:=*(*T5)(unsafe.Pointer(&x))")
	firstLane := json.RawMessage(`{"Copy":{"local":1,"projection":[{"ConstantIndex":{"offset":0,"from_end":false}}]}}`)
	g.simdIntrinsic("simd_insert", []json.RawMessage{constant(5, 8, 11, 22), constant(1, 4, 1), firstLane}, Place{Local: 1})
	g.line("return *(*[2]uint64)(unsafe.Pointer(&v1)) }")
	g.f = &Function{Body: &Body{Locals: []Local{{Type: 4}, {Type: 3}}}}
	g.frame = map[int]uint64{0: 0, 1: 0}
	g.line("func Widen(x [4]uint32) [4]uint64 { var storage [4]uint64; *(*[4]uint32)(unsafe.Pointer(&storage))=x; bp:=uintptr(unsafe.Pointer(&storage))")
	g.simdIntrinsic("simd_cast", []json.RawMessage{copy1}, Place{Local: 0})
	g.line("return storage }")
	g.frame = nil
	g.f = &Function{Body: &Body{Locals: []Local{{Type: 1}, {Type: 3}}}}
	g.line("func Reduce(x [4]uint32) uint32 { var v0 uint32; v1:=*(*T3)(unsafe.Pointer(&x))")
	g.simdIntrinsic("simd_reduce_or", []json.RawMessage{copy1}, Place{Local: 0})
	g.line("return v0 }")
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	testSIMDProgram(t, source, `package fixture
import "testing"
func TestValueOperands(t *testing.T) {
 if got:=Shuffle([2]uint64{41,42}); got!=[2]uint64{11,41} {t.Fatalf("shuffle constant/alias: %v",got)}
 if got:=Insert([2]uint64{41,42}); got!=[2]uint64{11,41} {t.Fatalf("insert scalar/destination alias: %v",got)}
 if got:=Widen([4]uint32{1,2,3,4}); got!=[4]uint64{1,2,3,4} {t.Fatalf("overlapping widening cast: %v",got)}
 if got:=Reduce([4]uint32{1,16,256,0x80000000}); got!=0x80000111 {t.Fatalf("reduction: %x",got)}
 requireNoGoAllocations(t,100,func(){Shuffle([2]uint64{41,42});Insert([2]uint64{41,42});Widen([4]uint32{1,2,3,4})})
}`)
}
