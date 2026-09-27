package fixture

import (
	"os"
	"strings"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func TestJSONMatchesNativeRust(t *testing.T) {
	data, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(want) != 6 {
		t.Fatal("incomplete native JSON reference")
	}
	floatWant := strings.Split(want[5], "|")
	if len(floatWant) != 2 || floatWant[0] == floatWant[1] {
		t.Fatal("native JSONValue oracle did not distinguish f32/field order")
	}
	if TypeEncodeOnly.JSON == nil || TypeEncodeOnly.JSONValue == nil || TypeEncodeOnly.FromJSON != nil || TypeDecodeOnly.JSON != nil || TypeDecodeOnly.JSONValue != nil || TypeDecodeOnly.FromJSON == nil || TypeNeither.JSON != nil || TypeNeither.JSONValue != nil || TypeNeither.FromJSON != nil {
		t.Fatal("JSON capability did not follow the concrete Rust trait predicates")
	}
	c := oxide.NewContext()
	defer c.Close()
	beforeContext := oxide.HeapStats().LiveAllocations
	packetInput := c.CopyString(want[0])
	decodeInput := c.CopyString(`{"value":17}`)
	borrowInput := c.CopyString(`"borrowed"`)
	badInput := c.CopyString(`{"title":7}`)
	mark := c.Mark()
	var resultTypes [6][3]*oxide.Type
	// Result descriptors are learned once from real calls. Subsequent calls must
	// retain exactly their result slot, including across temporary &str headers.
	call := func(kind, slot int, invoke func() oxide.Value) oxide.Value {
		entry := c.Mark()
		wantMark := entry
		if typ := resultTypes[kind][slot]; typ != nil {
			c.Alloc(typ.Size, typ.Align)
			wantMark = c.Mark()
			c.Restore(entry)
		}
		value := invoke()
		if typ := resultTypes[kind][slot]; typ == nil {
			resultTypes[kind][slot] = value.Type
		} else if value.Type != typ || c.Mark() != wantMark {
			t.Fatal("JSON operation changed type or retained temporary storage")
		}
		return value
	}
	invoke := func(kind int) {
		defer c.Restore(mark)
		switch kind {
		case 0:
			decoded := call(0, 0, func() oxide.Value { return TypePacket.FromJSON(c, packetInput) })
			if decoded.Variant() != "Ok" {
				t.Fatal("Packet JSON decode failed")
			}
			packet := decoded.Field("0")
			if packet.Field("title").String() != "oxide 雪" || packet.Field("count").Uint128() != (oxide.U128{Lo: 3, Hi: 1 << 36}) || packet.Field("values").Len() != 3 || packet.Field("values").Index(0).Int() != -7 {
				t.Fatal("deserialized Rust values changed")
			}
			materialized := call(0, 2, func() oxide.Value { return packet.Field("title").JSONValue(c) })
			if materialized.Variant() != "Ok" || packet.Field("title").String() != "oxide 雪" {
				t.Fatal("JSONValue consumed its owned source")
			}
			materialized.Drop(c)
			encoded := call(0, 1, func() oxide.Value { return packet.JSON(c) })
			if encoded.Variant() != "Ok" || encoded.Field("0").String() != want[0] {
				t.Fatal("Packet JSON serialization differs from native Rust")
			}
			frame := c.Mark()
			encoded.Drop(c)
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("JSON drop leaked a frame")
			}
		case 1:
			value := TypeEncodeOnly.Uninit(c)
			value.Field("value").SetUint(91)
			encoded := call(1, 0, func() oxide.Value { return value.JSON(c) })
			if encoded.Variant() != "Ok" || encoded.Field("0").String() != want[1] {
				t.Fatal("Serialize-only Rust type changed")
			}
			frame := c.Mark()
			encoded.Drop(c)
			value.Drop(c)
			if c.Mark() != frame {
				t.Fatal("Serialize drop frame")
			}
		case 2:
			decoded := call(2, 0, func() oxide.Value { return TypeDecodeOnly.FromJSON(c, decodeInput) })
			if decoded.Variant() != "Ok" || decoded.Field("0").Field("value").Uint() != 17 || want[2] != "17" {
				t.Fatal("Deserialize-only Rust type changed")
			}
			frame := c.Mark()
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("Deserialize drop frame")
			}
		case 3:
			decoded := call(3, 0, func() oxide.Value { return TypeBorrowed.FromJSON(c, borrowInput) })
			if decoded.Variant() != "Ok" {
				t.Fatal("borrowed JSON decode failed")
			}
			borrowed := decoded.Field("0").Deref()
			if borrowed.String() != want[3] || borrowed.Span().Data != borrowInput.Data+1 {
				t.Fatal("deserializer lost its borrow into the original input")
			}
			frame := c.Mark()
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("borrowed decode drop frame")
			}
		case 4:
			decoded := call(4, 0, func() oxide.Value { return TypePacket.FromJSON(c, badInput) })
			if decoded.Variant() != "Err" {
				t.Fatal("invalid JSON accepted")
			}
			message := call(4, 1, func() oxide.Value { return decoded.Field("0").Display(c) })
			if message.String() != want[4] {
				t.Fatal("JSON error differs from native Rust")
			}
			frame := c.Mark()
			message.Drop(c)
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("JSON error drop frame")
			}
		case 5:
			value := TypeFloatOrder.Uninit(c)
			value.Field("z").SetFloat(0.1)
			value.Field("a").SetUint(7)
			direct := call(5, 0, func() oxide.Value { return value.JSON(c) })
			materialized := call(5, 1, func() oxide.Value { return value.JSONValue(c) })
			if direct.Variant() != "Ok" || materialized.Variant() != "Ok" {
				t.Fatal("FloatOrder serialization failed")
			}
			encoded := call(5, 2, func() oxide.Value { return materialized.Field("0").JSON(c) })
			if encoded.Variant() != "Ok" || direct.Field("0").String() != floatWant[0] || encoded.Field("0").String() != floatWant[1] {
				t.Fatal("JSONValue did not preserve native f32/field-order semantics")
			}
			if value.Field("z").Float() != float64(float32(0.1)) || value.Field("a").Uint() != 7 {
				t.Fatal("JSONValue changed the borrowed source")
			}
			frame := c.Mark()
			encoded.Drop(c)
			materialized.Drop(c)
			direct.Drop(c)
			value.Drop(c)
			if c.Mark() != frame {
				t.Fatal("JSONValue drop frame")
			}
		default:
			t.Fatal("unknown JSON case")
		}
		if c.Failed() {
			t.Fatal("JSON operation escaped as a Rust panic")
		}
	}
	// Warm each fixed path before the heap baseline; never adapt it to growth.
	for kind := 0; kind < 6; kind++ {
		invoke(kind)
	}
	baseline := oxide.HeapStats().LiveAllocations
	for kind, name := range []string{"packet", "serialize_only", "deserialize_only", "borrowed", "error", "value_f32_order"} {
		t.Run(name, func(t *testing.T) {
			requireNoGoAllocations(t, 100, func() {
				invoke(kind)
				if c.Mark() != mark || c.Failed() {
					t.Fatal("JSON boundary frame/panic")
				}
				if live := oxide.HeapStats().LiveAllocations; live != baseline {
					t.Fatalf("JSON leaked Rust owners: %d -> %d", baseline, live)
				}
			})
		})
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if oxide.HeapStats().LiveAllocations != beforeContext {
		t.Fatal("JSON context closure retained Rust ownership")
	}
}
