// Package mir lowers the pinned rustc_public interchange emitted by oxide-rs.
// MIR is already type checked, monomorphized and drop-elaborated by rustc.
package mir

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type Program struct {
	Compiler      string                     `json:"compiler"`
	PanicStrategy string                     `json:"panic_strategy"`
	RuntimeChecks map[string]bool            `json:"runtime_checks"`
	Target        string                     `json:"target"`
	Roots         []Root                     `json:"roots"`
	Types         []Type                     `json:"types"`
	Functions     []Function                 `json:"functions"`
	Allocations   []json.RawMessage          `json:"allocations"`
	VTables       map[string]uint64          `json:"vtables"`
	Upcasts       map[string]int64           `json:"upcasts"`
	ThreadLocals  map[uint64]json.RawMessage `json:"thread_locals"`
}
type Allocation struct {
	ID    uint64
	Bytes []byte
	Align uint64
	Ptrs  []Relocation
}
type Relocation struct {
	Offset uint64
	Target uint64
}
type Root struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
}
type Type struct {
	ID                int         `json:"id"`
	Name              string      `json:"name"`
	Kind              string      `json:"kind"`
	Size              uint64      `json:"size"`
	Align             uint64      `json:"align"`
	Pack              uint64      `json:"pack"`
	Sized             bool        `json:"sized"`
	LayoutError       string      `json:"layout_error"`
	ValueABI          string      `json:"value_abi"`
	ABIScalar         *Primitive  `json:"abi_scalar"`
	ABIPair           *ScalarPair `json:"abi_pair"`
	Fields            []uint64    `json:"fields"`
	Variants          [][]uint64  `json:"variants"`
	FieldTypes        []int       `json:"field_types"`
	VariantFieldTypes [][]int     `json:"variant_field_types"`
	VariantNames      []string    `json:"variant_names"`
	AdtKind           string      `json:"adt_kind"`
	Pointee           int         `json:"pointee"`
	Element           int         `json:"element"`
	Length            uint64      `json:"length"`
	Function          string      `json:"function"`
	FnInputs          []int       `json:"fn_inputs"`
	FnOutput          int         `json:"fn_output"`
	FnABI             string      `json:"fn_abi"`
	FnVariadic        bool        `json:"fn_variadic"`
	FnFixedCount      int         `json:"fn_fixed_count"`
	Discriminants     []string    `json:"discriminants"`
	Variant           int         `json:"variant"`
	Tag               *Tag        `json:"tag"`
}
type Primitive struct {
	Int *struct {
		Length string `json:"length"`
		Signed bool   `json:"signed"`
	} `json:"Int"`
	Float *struct {
		Length string `json:"length"`
	} `json:"Float"`
	Pointer *uint64 `json:"Pointer"`
}
type ScalarPair struct {
	A       Primitive `json:"a"`
	B       Primitive `json:"b"`
	BOffset uint64    `json:"b_offset"`
}
type Tag struct {
	Offset    uint64 `json:"offset"`
	Size      uint64 `json:"size"`
	Primitive struct {
		Int struct {
			Signed bool `json:"signed"`
		} `json:"Int"`
	} `json:"primitive"`
	Encoding string `json:"encoding"`
	Start    string `json:"start"`
	First    int    `json:"first"`
	Last     int    `json:"last"`
	Untagged int    `json:"untagged"`
}
type Function struct {
	Symbol            string               `json:"symbol"`
	Name              string               `json:"name"`
	Kind              string               `json:"kind"`
	Intrinsic         *string              `json:"intrinsic"`
	EmptyDrop         bool                 `json:"empty_drop"`
	TrackCaller       bool                 `json:"track_caller"`
	Constructor       *int                 `json:"constructor"`
	Body              *Body                `json:"body"`
	Calls             map[int]string       `json:"calls"`
	CallLocations     map[int]CallLocation `json:"call_locations"`
	AssertCalls       map[int]AssertCall   `json:"assert_calls"`
	CallUntuple       map[int]int          `json:"call_untuple"`
	IntrinsicValidity map[int]bool         `json:"intrinsic_validity"`
	Signature         *Signature           `json:"signature"`
}
type CallLocation struct {
	Inherited  bool   `json:"inherited"`
	Allocation uint64 `json:"allocation"`
	Offset     uint64 `json:"offset"`
}
type AssertCall struct {
	Symbol   string            `json:"symbol"`
	Args     []json.RawMessage `json:"args"`
	Optional bool              `json:"optional"`
}
type Signature struct {
	Params     []int  `json:"params"`
	Return     int    `json:"return"`
	ABI        string `json:"abi"`
	Variadic   bool   `json:"variadic"`
	FixedCount int    `json:"fixed_count"`
	CanUnwind  bool   `json:"can_unwind"`
}

type External struct {
	Name         string `json:"name"`
	Symbol       string `json:"symbol"`
	Weak         bool   `json:"weak"`
	Linkage      string `json:"linkage"`
	Kind         string `json:"kind"`
	Type         int    `json:"type"`
	FunctionType int    `json:"function_type"`
}
type Body struct {
	Locals    []Local `json:"locals"`
	ArgCount  int     `json:"arg_count"`
	Blocks    []Block `json:"blocks"`
	SpreadArg *int    `json:"spread_arg"`
}
type Local struct {
	Type int `json:"ty"`
}
type Block struct {
	Statements []Statement `json:"statements"`
	Terminator Statement   `json:"terminator"`
}
type Statement struct {
	Kind json.RawMessage `json:"kind"`
}
type Place struct {
	Local      int               `json:"local"`
	Projection []json.RawMessage `json:"projection"`
}

func Load(path string) (*Program, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Program
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Compiler != "nightly-2026-09-15 (574ff7d98)" {
		return nil, fmt.Errorf("unsupported MIR producer %q", p.Compiler)
	}
	for _, t := range p.Types {
		if t.Sized && t.Size != 0 && t.LayoutError == "" && t.ValueABI == "" {
			return nil, fmt.Errorf("type %d is missing compiler value ABI", t.ID)
		}
	}
	return &p, nil
}

// GoArch prevents target-specific layouts and intrinsics from being compiled
// for another architecture merely because both use 64-bit pointers.
func (p *Program) GoArch() (string, error) {
	switch p.Target {
	case "x86_64-unknown-linux-gnu":
		return "amd64", nil
	case "aarch64-unknown-linux-gnu":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported MIR target %q", p.Target)
	}
}

func (p *Program) MemoryAllocations() ([]Allocation, error) {
	var out []Allocation
	for _, raw := range p.Allocations {
		var x struct {
			ID     uint64 `json:"id"`
			Memory *struct {
				Bytes      []*uint8 `json:"bytes"`
				Align      uint64   `json:"align"`
				Provenance struct {
					Ptrs [][]uint64 `json:"ptrs"`
				} `json:"provenance"`
			} `json:"memory"`
		}
		if err := json.Unmarshal(raw, &x); err != nil {
			return nil, err
		}
		if x.Memory == nil {
			continue
		}
		b := make([]byte, len(x.Memory.Bytes))
		for i, v := range x.Memory.Bytes {
			if v != nil {
				b[i] = *v
			}
		}
		a := Allocation{ID: x.ID, Bytes: b, Align: x.Memory.Align}
		for _, pair := range x.Memory.Provenance.Ptrs {
			if len(pair) == 2 {
				a.Ptrs = append(a.Ptrs, Relocation{pair[0], pair[1]})
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func WriteAllocations(p *Program, path string) error {
	a, err := p.MemoryAllocations()
	if err != nil {
		return err
	}
	sort.Slice(a, func(i, j int) bool { return a[i].ID < a[j].ID })
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write([]byte("OXAL1")); err != nil {
		return err
	}
	put := func(v uint64) error {
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], v)
		_, e := f.Write(b[:])
		return e
	}
	if err = put(uint64(len(a))); err != nil {
		return err
	}
	for _, x := range a {
		if err = put(x.ID); err != nil {
			return err
		}
		if err = put(uint64(len(x.Bytes))); err != nil {
			return err
		}
		if err = put(x.Align); err != nil {
			return err
		}
		if err = put(uint64(len(x.Ptrs))); err != nil {
			return err
		}
		if _, err = f.Write(x.Bytes); err != nil {
			return err
		}
		for _, r := range x.Ptrs {
			if err = put(r.Offset); err != nil {
				return err
			}
			if err = put(r.Target); err != nil {
				return err
			}
		}
	}
	return f.Sync()
}

func variant(b json.RawMessage) (string, json.RawMessage) {
	var name string
	if json.Unmarshal(b, &name) == nil {
		return name, nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil || len(m) != 1 {
		panic(fmt.Sprintf("invalid MIR enum %s", b))
	}
	for k, v := range m {
		return k, v
	}
	panic("unreachable")
}
func decode[T any](b json.RawMessage) T {
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		panic(err)
	}
	return v
}
