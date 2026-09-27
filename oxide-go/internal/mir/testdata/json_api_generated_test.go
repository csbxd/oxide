package fixture_test

import (
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	"os"
	api "oxide-json-conformance"
	"strings"
	"testing"
)

// Check the concrete result slot without a descriptor or interface dispatch.
func jsonResultFrame(t *testing.T, c *oxide.Context, entry oxide.Mark, size, align uintptr) {
	t.Helper()
	got := c.Mark()
	c.Restore(entry)
	c.Alloc(size, align)
	want := c.Mark()
	c.Restore(got)
	if got != want {
		t.Fatal("JSON operation retained temporary storage")
	}
}

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
	c := oxide.NewContext()
	defer c.Close()
	beforeContext := oxide.HeapStats().LiveAllocations
	packetInput := api.Borrow__Str(c.CopyString(want[0]))
	decodeInput := api.Borrow__Str(c.CopyString(`{"value":17}`))
	borrowInput := api.Borrow__Str(c.CopyString(`"borrowed"`))
	badInput := api.Borrow__Str(c.CopyString(`{"title":7}`))
	mark := c.Mark()
	invoke := func(kind int) {
		defer c.Restore(mark)
		switch kind {
		case 0:
			entry := c.Mark()
			decoded := api.FromJSON__Packet(c, packetInput)
			jsonResultFrame(t, c, entry, decoded.Size(), decoded.Align())
			if decoded.Ref().Variant().String() != "Ok" {
				t.Fatal("Packet JSON decode failed")
			}
			packet := decoded.Ref().Field__Ok__0()
			if packet.Field__Title().String() != "oxide 雪" || packet.Field__Count().Get() != (oxide.U128{Lo: 3, Hi: 1 << 36}) || packet.Field__Values().Len() != 3 || packet.Field__Values().Index(0).Get() != -7 {
				t.Fatal("deserialized Rust values changed")
			}
			entry = c.Mark()
			materialized := packet.Field__Title().JSONValue(c)
			jsonResultFrame(t, c, entry, materialized.Size(), materialized.Align())
			if materialized.Ref().Variant().String() != "Ok" || packet.Field__Title().String() != "oxide 雪" {
				t.Fatal("JSONValue consumed its owned source")
			}
			materialized.Drop(c)
			entry = c.Mark()
			encoded := packet.JSON(c)
			jsonResultFrame(t, c, entry, encoded.Size(), encoded.Align())
			if encoded.Ref().Variant().String() != "Ok" || encoded.Ref().Field__Ok__0().String() != want[0] {
				t.Fatal("Packet JSON serialization differs from native Rust")
			}
			frame := c.Mark()
			encoded.Drop(c)
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("JSON drop leaked a frame")
			}
		case 1:
			value := api.New__EncodeOnly(c)
			value.Mut().Field__Value().Set(91)
			entry := c.Mark()
			encoded := value.Ref().JSON(c)
			jsonResultFrame(t, c, entry, encoded.Size(), encoded.Align())
			if encoded.Ref().Variant().String() != "Ok" || encoded.Ref().Field__Ok__0().String() != want[1] {
				t.Fatal("Serialize-only Rust type changed")
			}
			frame := c.Mark()
			encoded.Drop(c)
			value.Drop(c)
			if c.Mark() != frame {
				t.Fatal("Serialize drop frame")
			}
		case 2:
			entry := c.Mark()
			decoded := api.FromJSON__DecodeOnly(c, decodeInput)
			jsonResultFrame(t, c, entry, decoded.Size(), decoded.Align())
			if decoded.Ref().Variant().String() != "Ok" || decoded.Ref().Field__Ok__0().Field__Value().Get() != 17 || want[2] != "17" {
				t.Fatal("Deserialize-only Rust type changed")
			}
			frame := c.Mark()
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("Deserialize drop frame")
			}
		case 3:
			entry := c.Mark()
			decoded := api.FromJSON__Borrowed(c, borrowInput)
			jsonResultFrame(t, c, entry, decoded.Size(), decoded.Align())
			if decoded.Ref().Variant().String() != "Ok" {
				t.Fatal("borrowed JSON decode failed")
			}
			borrowed := decoded.Ref().Field__Ok__0().Deref()
			if borrowed.String() != want[3] || borrowed.Span().Data != borrowInput.Addr()+1 {
				t.Fatal("deserializer lost its original-input borrow")
			}
			frame := c.Mark()
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("borrowed decode drop frame")
			}
		case 4:
			entry := c.Mark()
			decoded := api.FromJSON__Packet(c, badInput)
			jsonResultFrame(t, c, entry, decoded.Size(), decoded.Align())
			if decoded.Ref().Variant().String() != "Err" {
				t.Fatal("invalid JSON accepted")
			}
			entry = c.Mark()
			message := decoded.Ref().Field__Err__0().Display(c)
			jsonResultFrame(t, c, entry, message.Size(), message.Align())
			if message.Ref().String() != want[4] {
				t.Fatal("JSON error differs from native Rust")
			}
			frame := c.Mark()
			message.Drop(c)
			decoded.Drop(c)
			if c.Mark() != frame {
				t.Fatal("JSON error drop frame")
			}
		case 5:
			value := api.New__FloatOrder(c)
			value.Mut().Field__Z().Set(0.1)
			value.Mut().Field__A().Set(7)
			entry := c.Mark()
			direct := value.Ref().JSON(c)
			jsonResultFrame(t, c, entry, direct.Size(), direct.Align())
			entry = c.Mark()
			materialized := value.Ref().JSONValue(c)
			jsonResultFrame(t, c, entry, materialized.Size(), materialized.Align())
			if direct.Ref().Variant().String() != "Ok" || materialized.Ref().Variant().String() != "Ok" {
				t.Fatal("FloatOrder serialization failed")
			}
			entry = c.Mark()
			encoded := materialized.Ref().Field__Ok__0().JSON(c)
			jsonResultFrame(t, c, entry, encoded.Size(), encoded.Align())
			if encoded.Ref().Variant().String() != "Ok" || direct.Ref().Field__Ok__0().String() != floatWant[0] || encoded.Ref().Field__Ok__0().String() != floatWant[1] {
				t.Fatal("JSONValue did not preserve native f32/field-order semantics")
			}
			if value.Ref().Field__Z().Get() != float32(0.1) || value.Ref().Field__A().Get() != 7 {
				t.Fatal("JSONValue changed its source")
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
