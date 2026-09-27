package mir

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The niche starts at u128::MAX and spans zero. This also covers layouts that
// rustc may choose only for some versions of a dependency or compiler.
func TestTagLowering(t *testing.T) {
	g := &generator{types: make(map[int]*Type)}
	g.line("package fixture\nimport (oxide \"github.com/csbxd/oxide/oxide-go/runtime\"; \"unsafe\")")
	const max128 = "340282366920938463463374607431768211455"
	const negative7 = "340282366920938463463374607431768211449"
	const high = "1267650600228229401496703205379"
	for _, tc := range []struct {
		name          string
		size          uint64
		encoding      string
		start         string
		signed        bool
		discriminants []string
	}{
		{"Niche128", 16, "niche", max128, false, []string{"0", "1", "2"}},
		{"Niche8", 1, "niche", "255", false, []string{"0", "1", "2"}},
		{"Signed8", 1, "direct", "", true, []string{negative7, "42"}},
		{"Signed128", 16, "direct", "", true, []string{negative7, high}},
	} {
		tag := &Tag{Size: tc.size, Encoding: tc.encoding, Start: tc.start, First: 0, Last: 1, Untagged: 2}
		tag.Primitive.Int.Signed = tc.signed
		g.types[1] = &Type{ID: 1, Kind: "aggregate", Size: 16, Sized: true, Tag: tag, Discriminants: tc.discriminants}
		for i := 0; i < 2; i++ {
			g.line("func %sSet%d(value oxide.U128) oxide.U128 {", tc.name, i)
			g.setTag(location{g: g, typ: 1, address: "unsafe.Pointer(&value)"}, i)
			g.line("return value }")
		}
		for _, kind := range []string{"i64", "i128"} {
			size := uint64(8)
			if kind == "i128" {
				size = 16
			}
			g.types[2] = &Type{ID: 2, Kind: kind, Size: size, Sized: true}
			g.line("func %sRead%s(value oxide.U128) %s { var out %s", tc.name, kind, g.goType(2), g.goType(2))
			g.discriminant(location{g: g, typ: 2, local: "out"}, location{g: g, typ: 1, address: "unsafe.Pointer(&value)"})
			g.line("return out }")
		}
	}
	for _, tc := range []struct {
		name  string
		kind  string
		size  uint64
		first string
		next  string
	}{
		{"Switch8", "i8", 1, "249", "255"},
		{"Switch128", "u128", 16, max128, high},
		{"SwitchSigned128", "i128", 16, negative7, high},
	} {
		g.types[1] = &Type{ID: 1, Kind: tc.kind, Size: tc.size, Sized: true}
		g.f = &Function{Body: &Body{Locals: []Local{{Type: 1}, {Type: 1}}}}
		g.line("func %s(v1 %s) int {", tc.name, g.goType(1))
		term := fmt.Sprintf(`{"SwitchInt":{"discr":{"Copy":{"local":1,"projection":[]}},"targets":{"branches":[[%q,1],[%q,2]],"otherwise":3}}}`, tc.first, tc.next)
		g.terminator(0, json.RawMessage(term), 1)
		g.line("bb1: return 11; bb2: return 22; bb3: return 33 }")
	}
	check := `package fixture
import (
 "testing"
 oxide "github.com/csbxd/oxide/oxide-go/runtime"
)
func TestGeneratedTags(t *testing.T) {
 max := oxide.U128{Lo:^uint64(0),Hi:^uint64(0)}
 for i, value := range []oxide.U128{max, {}, {Lo:1}, {Hi:1}} {
  want := int64(i)
  if want > 2 { want=2 }
  if got:=Niche128Readi64(value); got!=want { t.Fatalf("niche128: %v => %d, want %d",value,got,want) }
  if got:=Niche128Readi128(value); got!=oxide.I128From64(want) { t.Fatalf("niche128 wide: %v => %v",value,got) }
 }
 if got:=Niche128Set0(oxide.U128{}); got!=max { t.Fatalf("niche128 max: %v",got) }
 if got:=Niche128Set1(max); got!=(oxide.U128{}) { t.Fatalf("niche128 wrap: %v",got) }
 for i, value := range []oxide.U128{{Lo:255}, {}, {Lo:1}} {
  if got:=Niche8Readi64(value); got!=int64(i) { t.Fatalf("niche8: %v => %d",value,got) }
 }
 if got:=Niche8Set1(oxide.U128{Lo:255}); got.Lo!=0 { t.Fatalf("niche8 wrap: %v",got) }
 signed:=Signed8Set0(oxide.U128{})
 if signed.Lo!=249 || Signed8Readi64(signed)!=-7 || Signed8Readi128(signed)!=oxide.I128From64(-7) { t.Fatalf("signed8 tag: %v",signed) }
 signed=Signed8Set1(oxide.U128{})
 if Signed8Readi64(signed)!=42 { t.Fatalf("signed8 positive: %v",signed) }
 negative:=oxide.I128From64(-7)
 high:=oxide.I128{Lo:3,Hi:1<<36}
 if got:=Signed128Set0(oxide.U128{}); got!=oxide.U128(negative) { t.Fatalf("signed128 negative: %v",got) }
 if got:=Signed128Set1(oxide.U128{}); got!=oxide.U128(high) { t.Fatalf("signed128 high: %v",got) }
 if Signed128Readi128(oxide.U128(negative))!=negative || Signed128Readi128(oxide.U128(high))!=high { t.Fatal("signed128 discriminant") }
 if Switch8(-7)!=11 || Switch8(-1)!=22 || Switch8(0)!=33 { t.Fatal("signed8 switch") }
 if Switch128(max)!=11 || Switch128(oxide.U128(high))!=22 || Switch128(oxide.U128{})!=33 { t.Fatal("unsigned128 switch") }
 if SwitchSigned128(negative)!=11 || SwitchSigned128(high)!=22 || SwitchSigned128(oxide.I128{})!=33 { t.Fatal("signed128 switch") }
}
`
	dir := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":  fmt.Sprintf("module oxide-tag-test\n\ngo 1.27.1\n\nrequire github.com/csbxd/oxide/oxide-go v0.0.0\nreplace github.com/csbxd/oxide/oxide-go => %q\n", module),
		"tags.go": g.b.String(), "tags_test.go": check,
	}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "-mod=mod", "-count=1", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "GOWORK=off", "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated tags: %v\n%s\n%s", err, output, g.b.String())
	}
}
