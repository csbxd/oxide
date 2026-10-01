package mir

import (
	"encoding/json"
	"fmt"
	"go/format"
	"testing"
)

func TestSIMD128MaskConsumers(t *testing.T) {
	g := &generator{types: map[int]*Type{
		1: {ID: 1, Kind: "f128", Sized: true, Size: 16, Align: 16},
		2: {ID: 2, Kind: "i128", Sized: true, Size: 16, Align: 16},
		3: {ID: 3, Kind: "u128", Sized: true, Size: 16, Align: 16},
		4: {ID: 4, Kind: "i32", Sized: true, Size: 4, Align: 4},
		5: {ID: 5, Kind: "bool", Sized: true, Size: 1, Align: 1},
		6: {ID: 6, Kind: "u8", Sized: true, Size: 1, Align: 1},
	}}
	g.line("package fixture\nimport(\"unsafe\";oxide \"github.com/csbxd/oxide/oxide-go/runtime\")")
	for _, id := range []int{1, 2, 3, 4} {
		e := g.typ(id)
		v := &Type{ID: id + 10, Kind: "array", Sized: true, Size: e.Size * 2, Align: e.Align, Element: id, Length: 2}
		g.types[v.ID] = v
		g.typeDecl(v)
	}
	operand := func(local int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"Copy":{"local":%d,"projection":[]}}`, local))
	}
	g.f = &Function{Body: &Body{Locals: []Local{{Type: 12}, {Type: 11}, {Type: 11}}}}
	g.line("func Compare(a,b [2]oxide.F128) T12 { var v0 T12; v1:=*(*T11)(unsafe.Pointer(&a)); v2:=*(*T11)(unsafe.Pointer(&b))")
	g.simdIntrinsic("simd_lt", []json.RawMessage{operand(1), operand(2)}, Place{Local: 0})
	g.line("return v0 }")
	for _, elem := range []int{2, 3, 4} {
		vector := elem + 10
		for _, op := range []string{"simd_reduce_all", "simd_reduce_any", "simd_bitmask", "simd_reduce_or", "simd_reduce_min", "simd_reduce_max", "simd_and", "simd_or", "simd_xor"} {
			ret := elem
			args := []json.RawMessage{operand(1)}
			switch op {
			case "simd_reduce_all", "simd_reduce_any":
				ret = 5
			case "simd_bitmask":
				ret = 6
			case "simd_and", "simd_or", "simd_xor":
				ret = vector
				args = append(args, operand(2))
			}
			g.f = &Function{Body: &Body{Locals: []Local{{Type: ret}, {Type: vector}, {Type: vector}}}}
			g.line("func Mask%d_%s(v1,v2 T%d) %s {var v0 %s", elem, op, vector, g.goType(ret), g.goType(ret))
			g.simdIntrinsic(op, args, Place{Local: 0})
			g.line("return v0 }")
		}
		for _, data := range []int{11, 14} {
			for _, alias := range []bool{false, true} {
				dst, suffix := 0, ""
				if alias {
					dst, suffix = 2, "Alias"
				}
				g.f = &Function{Body: &Body{Locals: []Local{{Type: data}, {Type: vector}, {Type: data}, {Type: data}}}}
				g.line("func Select%d_%d%s(v1 T%d,v2,v3 T%d) T%d {var v0 T%d; _=v0", elem, data, suffix, vector, data, data, data)
				g.simdIntrinsic("simd_select", []json.RawMessage{operand(1), operand(2), operand(3)}, Place{Local: dst})
				g.line("return v%d }", dst)
			}
		}
	}
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatal(err)
	}
	testSIMDProgram(t, source, `package fixture
import("testing";"unsafe";oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestMasks(t *testing.T) {
 trueValues:=[2]oxide.F128{{Lo:0x1234,Hi:0x7fff000000000000},{Hi:1<<63}}
 falseValues:=[2]oxide.F128{{Lo:0x9876,Hi:0xffff800000000000},{}}
 a,b:=*(*T11)(unsafe.Pointer(&trueValues)),*(*T11)(unsafe.Pointer(&falseValues))
 da,db:=[2]int32{11,22},[2]int32{33,44}
 va,vb:=*(*T14)(unsafe.Pointer(&da)),*(*T14)(unsafe.Pointer(&db))
 for pattern:=0;pattern<4;pattern++ {
  x:=[2]oxide.F128{oxide.F64ToF128(1),oxide.F64ToF128(1)}
  want,want32:=falseValues,db
  for i:=range x {if pattern>>i&1!=0 {x[i]=oxide.F64ToF128(-1);want[i]=trueValues[i];want32[i]=da[i]}}
  m:=Compare(x,[2]oxide.F128{{},{}})
  requireNoGoAllocations(t,100,func(){
   for _,f:=range []func(T12,T11,T11)T11{Select2_11,Select2_11Alias} {v:=f(m,a,b);if *(*[2]oxide.F128)(unsafe.Pointer(&v))!=want {t.Fatal("signed select",pattern)}}
   for _,f:=range []func(T12,T14,T14)T14{Select2_14,Select2_14Alias} {v:=f(m,va,vb);if *(*[2]int32)(unsafe.Pointer(&v))!=want32 {t.Fatal("mixed-width select",pattern)}}
   if Mask2_simd_reduce_all(m,m)!=(pattern==3)||Mask2_simd_reduce_any(m,m)!=(pattern!=0)||Mask2_simd_bitmask(m,m)!=uint8(pattern){t.Fatal("signed mask consumers",pattern)}
   zero:=Mask2_simd_xor(m,m);if Mask2_simd_reduce_any(zero,zero){t.Fatal("xor")}
   if Mask2_simd_or(m,zero)!=m || Mask2_simd_and(m,m)!=m {t.Fatal("signed mask operations")}
   or:=Mask2_simd_reduce_or(m,m);if (or!=oxide.I128{})!=(pattern!=0){t.Fatal("signed reduction")}
   if Mask2_simd_reduce_min(m,m)!=or || (Mask2_simd_reduce_max(m,m)!=oxide.I128{})!=(pattern==3){t.Fatal("signed min/max")}
   u:=*(*T13)(unsafe.Pointer(&m))
   for _,f:=range []func(T13,T11,T11)T11{Select3_11,Select3_11Alias} {v:=f(u,a,b);if *(*[2]oxide.F128)(unsafe.Pointer(&v))!=want {t.Fatal("unsigned select",pattern)}}
   if Mask3_simd_reduce_all(u,u)!=(pattern==3)||Mask3_simd_reduce_any(u,u)!=(pattern!=0)||Mask3_simd_bitmask(u,u)!=uint8(pattern){t.Fatal("unsigned mask consumers",pattern)}
   uz:=Mask3_simd_xor(u,u);if Mask3_simd_reduce_any(uz,uz)||Mask3_simd_or(u,uz)!=u||Mask3_simd_and(u,u)!=u{t.Fatal("unsigned mask operations")}
   uor:=Mask3_simd_reduce_or(u,u);if (uor!=oxide.U128{})!=(pattern!=0)||Mask3_simd_reduce_max(u,u)!=uor||(Mask3_simd_reduce_min(u,u)!=oxide.U128{})!=(pattern==3){t.Fatal("unsigned reductions")}
   var small [2]int32;for i:=range small {if pattern>>i&1!=0 {small[i]=-1}}
   sm:=*(*T14)(unsafe.Pointer(&small))
   v:=Select4_11(sm,a,b);if *(*[2]oxide.F128)(unsafe.Pointer(&v))!=want||Mask4_simd_reduce_any(sm,sm)!=(pattern!=0)||Mask4_simd_reduce_all(sm,sm)!=(pattern==3)||Mask4_simd_bitmask(sm,sm)!=uint8(pattern){t.Fatal("narrow mask",pattern)}
  })
 }
 // NaN is unordered; selection must preserve the chosen payload bits.
 m:=Compare([2]oxide.F128{trueValues[0],oxide.F64ToF128(-1)},[2]oxide.F128{{},{}})
 v:=Select2_11(m,a,b);if *(*[2]oxide.F128)(unsafe.Pointer(&v))!=([2]oxide.F128{falseValues[0],trueValues[1]}) {t.Fatal("NaN compare/select")}
 // Integer reductions/bitwise operations must retain bits in both words.
 raw:=[2]oxide.I128{{Lo:1},{Hi:2}};m=*(*T12)(unsafe.Pointer(&raw))
 if Mask2_simd_reduce_or(m,m)!=(oxide.I128{Lo:1,Hi:2}){t.Fatal("reduction lost a word")}
}`)
}
