package rendererfixture_test

import (
	"bytes"
	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	. "oxide-renderer-conformance"
	"strings"
)

// These constraints bind concrete generated Go methods. No Rust value is erased
// into an interface or inspected through runtime type metadata.
type apiTag interface {
	~uint32
	String() string
}
type apiPlace interface{ Addr() uintptr }
type apiDebug interface {
	Debug(*oxide.Context) Value__Alloc_String_String
}
type apiDisplay interface {
	Display(*oxide.Context) Value__Alloc_String_String
}
type apiResultView[T apiTag, O apiPlace, E apiDisplay] interface {
	Variant() T
	Field__Ok__0() O
	Field__Err__0() E
}
type apiOwner[R apiPlace] interface {
	Ref() R
	Drop(*oxide.Context)
}

func apiOK[T apiTag, O apiPlace, E apiDisplay, R interface {
	apiPlace
	apiResultView[T, O, E]
}, V apiOwner[R]](ctx *oxide.Context, value V) O {
	v := value.Ref()
	if v.Variant().String() == "Ok" {
		return v.Field__Ok__0()
	}
	text := v.Field__Err__0().Display(ctx)
	message := strings.Clone(text.Ref().String())
	text.Drop(ctx)
	panic(message)
}

func appendDebug[T apiDebug](ctx *oxide.Context, dst []byte, value T) []byte {
	text := value.Debug(ctx)
	dst = append(dst, text.Ref().Bytes()...)
	text.Drop(ctx)
	return dst
}

func appendError[T apiDisplay](ctx *oxide.Context, dst []byte, value T, path oxide.Span) []byte {
	text := value.Display(ctx)
	defer text.Drop(ctx)
	dst = append(dst, "error:"...)
	b := text.Ref().Bytes()
	for {
		i := bytes.Index(b, path.Bytes())
		if i < 0 {
			return append(dst, b...)
		}
		dst = append(dst, b[:i]...)
		dst = append(dst, "<path>"...)
		b = b[i+int(path.Len):]
	}
}

func appendStringResult[T apiTag, E apiDisplay, R interface {
	apiPlace
	apiResultView[T, Ref__Alloc_String_String, E]
}, V apiOwner[R]](ctx *oxide.Context, dst []byte, value V, path oxide.Span) []byte {
	defer value.Drop(ctx)
	v := value.Ref()
	if v.Variant().String() == "Err" {
		return appendError(ctx, dst, v.Field__Err__0(), path)
	}
	return append(dst, v.Field__Ok__0().Bytes()...)
}

func appendDebugResult[T apiTag, O interface {
	apiPlace
	apiDebug
}, E apiDisplay, R interface {
	apiPlace
	apiResultView[T, O, E]
}, V apiOwner[R]](ctx *oxide.Context, dst []byte, value V, path oxide.Span) []byte {
	defer value.Drop(ctx)
	v := value.Ref()
	if v.Variant().String() == "Err" {
		return appendError(ctx, dst, v.Field__Err__0(), path)
	}
	return appendDebug(ctx, dst, v.Field__Ok__0())
}

func appendJSON[T interface {
	JSON(*oxide.Context) Value__Core_Result_Result__Of__Alloc_String_String__And__SerdeJson_Error_Error__End
}](ctx *oxide.Context, dst []byte, value T) []byte {
	result := value.JSON(ctx)
	defer result.Drop(ctx)
	return append(dst, apiOK(ctx, result).Bytes()...)
}

// json! follows this real Rust to_value path, including f32 widening and object
// key order; serializing the original T directly is a different observable API.
func appendJSONValue[T interface {
	JSONValue(*oxide.Context) Value__Core_Result_Result__Of__SerdeJson_Value_Value__And__SerdeJson_Error_Error__End
}](ctx *oxide.Context, dst []byte, value T) []byte {
	result := value.JSONValue(ctx)
	defer result.Drop(ctx)
	return appendJSON(ctx, dst, apiOK(ctx, result))
}

func appendParsed[T apiTag, E apiDisplay, R interface {
	apiPlace
	apiResultView[T, Ref__ParseOutput, E]
}, V apiOwner[R]](ctx *oxide.Context, dst []byte, result V, path oxide.Span) []byte {
	defer result.Drop(ctx)
	v := result.Ref()
	if v.Variant().String() == "Err" {
		return appendError(ctx, dst, v.Field__Err__0(), path)
	}
	parsed := v.Field__Ok__0()
	dst = appendGraph(ctx, dst, parsed.Field__Graph())
	return appendDebug(ctx, dst, parsed.Field__InitConfig())
}

func layoutValues(ctx *oxide.Context, source oxide.Span) (Value__Core_Result_Result__Of__ParseOutput__And__Anyhow_Error__End, Value__Theme, Value__LayoutConfig, Value__Layout) {
	parsed := ParseMermaid(ctx, Borrow__Str(source))
	theme := Theme_Modern(ctx)
	config := Default__LayoutConfig(ctx)
	layout := ComputeLayout(ctx, apiOK(ctx, parsed).Field__Graph(), theme.Ref(), config.Ref())
	return parsed, theme, config, layout
}

func someFloat(ctx *oxide.Context, value float32) Value__Core_Option_Option__Of__F32__End {
	return New__Core_Option_Option__Of__F32__End__Some(ctx, value)
}
func dimensions(ctx *oxide.Context, width, height float32) Value__Core_Option_Option__Of__Tuple__Of__F32__And__F32__End__End {
	v := New__Tuple__Of__F32__And__F32__End(ctx)
	v.Mut().Field__0().Set(width)
	v.Mut().Field__1().Set(height)
	return New__Core_Option_Option__Of__Tuple__Of__F32__And__F32__End__End__Some(ctx, v)
}
func optionalPath(ctx *oxide.Context, span oxide.Span, some bool) Value__Core_Option_Option__Of__Ref__Of__Std_Path_Path__End__End {
	if !some {
		return New__Core_Option_Option__Of__Ref__Of__Std_Path_Path__End__End__None(ctx)
	}
	return New__Core_Option_Option__Of__Ref__Of__Std_Path_Path__End__End__Some(ctx, Borrow__Std_Path_Path(span))
}
func optionalStr(ctx *oxide.Context, span oxide.Span, some bool) Value__Core_Option_Option__Of__Ref__Of__Str__End__End {
	if !some {
		return New__Core_Option_Option__Of__Ref__Of__Str__End__End__None(ctx)
	}
	return New__Core_Option_Option__Of__Ref__Of__Str__End__End__Some(ctx, Borrow__Str(span))
}
