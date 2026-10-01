package mir

import (
	"encoding/json"
	"fmt"
	"go/format"
	"testing"
)

func TestSoftFloatSIMDAndMathProvider(t *testing.T) {
	g := &generator{types: map[int]*Type{
		16:  {ID: 16, Kind: "f16", Size: 2, Align: 2, Sized: true},
		128: {ID: 128, Kind: "f128", Size: 16, Align: 16, Sized: true},
		8:   {ID: 8, Kind: "pointer", Size: 8, Align: 8, Sized: true},
	}}
	g.line("package fixture\nimport(\"unsafe\"; oxide \"github.com/csbxd/oxide/oxide-go/runtime\")")
	copy1 := json.RawMessage(`{"Copy":{"local":1,"projection":[]}}`)
	copy2 := json.RawMessage(`{"Copy":{"local":2,"projection":[]}}`)
	for _, width := range []int{16, 128} {
		array := width + 1
		g.types[array] = &Type{ID: array, Kind: "array", Element: width, Length: 2, Size: uint64(width / 4), Align: uint64(width / 8), Sized: true}
		g.typeDecl(g.types[array])
		integer, mask := width+1000, width+1001
		g.types[integer] = &Type{ID: integer, Kind: fmt.Sprintf("i%d", width), Size: uint64(width / 8), Align: uint64(width / 8), Sized: true}
		g.types[mask] = &Type{ID: mask, Kind: "array", Element: integer, Length: 2, Size: uint64(width / 4), Align: uint64(width / 8), Sized: true}
		g.typeDecl(g.types[mask])
		for _, op := range []string{"simd_neg", "simd_fabs", "simd_fsqrt", "simd_add", "simd_reduce_min", "simd_lt"} {
			ret := array
			if op == "simd_reduce_min" {
				ret = width
			}
			if op == "simd_lt" {
				ret = mask
			}
			g.f = &Function{Body: &Body{Locals: []Local{{Type: ret}, {Type: array}, {Type: array}}}}
			g.line("func Op%d_%s(a,b [2]oxide.F%d) %s {v1:=*(*T%d)(unsafe.Pointer(&a));v2:=*(*T%d)(unsafe.Pointer(&b));_=v2; var v0 %s", width, op, width, g.goType(ret), array, array, g.goType(ret))
			args := []json.RawMessage{copy1}
			if op == "simd_add" || op == "simd_lt" {
				args = append(args, copy2)
			}
			g.simdIntrinsic(op, args, Place{Local: 0})
			g.line("return v0 }")
		}
	}
	for _, pair := range [][2]int{{17, 129}, {129, 17}, {129, 1129}} {
		g.f = &Function{Body: &Body{Locals: []Local{{Type: pair[1]}, {Type: pair[0]}}}}
		g.line("func Cast%dTo%d(v1 T%d) T%d {var v0 T%d", pair[0], pair[1], pair[0], pair[1], pair[1])
		g.simdIntrinsic("simd_cast", []json.RawMessage{copy1}, Place{Local: 0})
		g.line("return v0 }")
	}
	for _, op := range []string{"exp", "powf128"} {
		g.f = &Function{Body: &Body{Locals: []Local{{Type: 128}, {Type: 128}, {Type: 128}}}}
		g.line("func Hook%s(ctx *oxide.Context,v1,v2 oxide.F128) oxide.F128 {var v0 oxide.F128", op)
		args := []json.RawMessage{copy1}
		if op == "powf128" {
			args = append(args, copy2)
		}
		if !g.softFloatIntrinsic(op, args, Place{Local: 0}) {
			t.Fatal("missing intrinsic", op)
		}
		g.line("return v0 }")
	}
	for _, symbol := range []string{"sinf128", "lgammaf128_r"} {
		params := []int{128}
		decl := "v1 oxide.F128"
		if symbol == "lgammaf128_r" {
			params = append(params, 8)
			decl += ",v2 uintptr"
		}
		f := &Function{Symbol: symbol, Signature: &Signature{ABI: "C", Params: params, Return: 128}}
		g.line("func C%s(ctx *oxide.Context,%s) oxide.F128 {", symbol, decl)
		if !g.cRuntimeFunction(f, params, 128) {
			t.Fatal("missing C binding", symbol)
		}
	}
	// Scalar-pair constants must retain full bits and real component offsets.
	var f16, f128 Primitive
	if err := json.Unmarshal([]byte(`{"Float":{"length":"F16"}}`), &f16); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"Float":{"length":"F128"}}`), &f128); err != nil {
		t.Fatal(err)
	}
	quadBytes := make([]byte, 16)
	quadBytes[0] = 1
	quadBytes[14] = 255
	quadBytes[15] = 127
	g.line("func Constants() (oxide.F16,oxide.F128) {return %s,%s}", g.primitiveConstant(f16, []byte{1, 124}), g.primitiveConstant(f128, quadBytes))
	pair := &Type{ID: 300, Name: "(f16,f128)", Kind: "aggregate", Size: 32, Align: 16, Sized: true, ValueABI: "ScalarPair", ABIPair: &ScalarPair{A: f16, B: f128, BOffset: 16}}
	g.types[300] = pair
	g.scalarPairDecl(pair)
	pairBytes := make([]byte, 32)
	pairBytes[0] = 1
	pairBytes[1] = 124
	copy(pairBytes[16:], quadBytes)
	g.line("func PairConstant() T300 {return %s}", g.scalarPairConstant(pair, pairBytes))
	source, err := format.Source([]byte(g.b.String()))
	if err != nil {
		t.Fatalf("%v\n%s", err, g.b.String())
	}
	check := `package fixture
import("testing";"unsafe";oxide "github.com/csbxd/oxide/oxide-go/runtime")
func TestGenerated(t *testing.T) {
 ctx:=oxide.NewContext();defer ctx.Close()
 ctx.Binary128Math=func(c *oxide.Context,op string,x,y oxide.F128)(oxide.F128,int32){
  if c!=ctx {t.Fatal("provider context")}
  if op=="pow" {return y,0};if op=="lgamma" {return x,-1};if op!="sin" && op!="exp" {t.Fatal(op)};return x,0
 }
 a,b:=oxide.F128{Lo:1,Hi:0x7fff000000000000},oxide.F128{Lo:2,Hi:0x3fff000000000000}
 p:=ctx.Alloc(12,4);*(*uint32)(unsafe.Pointer(p))=123;*(*uint32)(unsafe.Pointer(p+8))=456
 requireNoGoAllocations(t,100,func(){
  if Hookexp(ctx,a,b)!=a || Hookpowf128(ctx,a,b)!=b || Csinf128(ctx,a)!=a || Clgammaf128_r(ctx,a,p+4)!=a {t.Fatal("math provider narrowed bits")}
  if *(*int32)(unsafe.Pointer(p+4))!=-1 || *(*uint32)(unsafe.Pointer(p))!=123 || *(*uint32)(unsafe.Pointer(p+8))!=456 {t.Fatal("lgamma sign storage")}
  h,q:=Constants();if h!=0x7c01 || q!=a {t.Fatal("ABI constants")}
  pair:=PairConstant();if pair.A!=h || pair.B!=q || unsafe.Offsetof(pair.B)!=16 {t.Fatal("ABI scalar pair")}
  x:=[2]oxide.F16{0x8000,0x7c01};got:=Op16_simd_neg(x,x);if *(*[2]oxide.F16)(unsafe.Pointer(&got))!=([2]oxide.F16{0,0xfc01}) {t.Fatal("half sign bits")}
  y:=[2]oxide.F128{{Hi:0x8000000000000000},a};wide:=Op128_simd_fabs(y,y);if *(*[2]oxide.F128)(unsafe.Pointer(&wide))!=([2]oxide.F128{{},a}) {t.Fatal("quad sign bits")}
  x=[2]oxide.F16{0x4400,0x4880};r:=Op16_simd_fsqrt(x,x);if *(*[2]oxide.F16)(unsafe.Pointer(&r))!=([2]oxide.F16{0x4000,0x4200}) {t.Fatal("half sqrt")}
  y=[2]oxide.F128{oxide.F64ToF128(4),oxide.F64ToF128(9)};s:=Op128_simd_fsqrt(y,y);if *(*[2]oxide.F128)(unsafe.Pointer(&s))!=([2]oxide.F128{oxide.F64ToF128(2),oxide.F64ToF128(3)}) {t.Fatal("quad sqrt")}
  if Op16_simd_reduce_min(x,x)!=0x4400 || Op128_simd_reduce_min(y,y)!=y[0] {t.Fatal("numeric reduction")}
  sum16:=Op16_simd_add(x,x);if *(*[2]oxide.F16)(unsafe.Pointer(&sum16))!=([2]oxide.F16{0x4800,0x4c80}) {t.Fatal("half addition")}
  sum128:=Op128_simd_add(y,y);if *(*[2]oxide.F128)(unsafe.Pointer(&sum128))!=([2]oxide.F128{oxide.F64ToF128(8),oxide.F64ToF128(18)}) {t.Fatal("quad addition")}
  cmp16:=Op16_simd_lt([2]oxide.F16{0xbc00,0x7e00},[2]oxide.F16{0x3c00,0});if *(*[2]int16)(unsafe.Pointer(&cmp16))!=([2]int16{-1,0}) {t.Fatal("half comparison mask")}
  cmp128:=Op128_simd_lt([2]oxide.F128{oxide.F64ToF128(-1),a},[2]oxide.F128{{},{}});if *(*[2]oxide.I128)(unsafe.Pointer(&cmp128))!=([2]oxide.I128{oxide.I128From64(-1),{}}) {t.Fatal("quad comparison mask")}
  extended:=Cast17To129(sum16);narrowed:=Cast129To17(extended);if narrowed!=sum16 {t.Fatal("SIMD float casts")}
  integers:=Cast129To1129(sum128);if *(*[2]oxide.I128)(unsafe.Pointer(&integers))!=([2]oxide.I128{oxide.I128From64(8),oxide.I128From64(18)}) {t.Fatal("SIMD integer casts")}
 })
 ctx.Binary128Math=nil
 if got:=Hookexp(ctx,oxide.F128{},b);got!=oxide.F64ToF128(1) {t.Fatal("built-in math",got)}
}
`
	testSIMDProgram(t, source, check)
}
