package main

import (
	"fmt"
	"os"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	renderer "oxide-renderer-conformance"
)

func main() {
	c := oxide.NewContext()
	defer c.Close()
	result := renderer.Run(c)
	status := 0
	if result.Ref().Variant() == renderer.Variant__Core_Result_Result__Of__Unit__And__Anyhow_Error__End__Err {
		message := result.Ref().Field__Err__0().Display(c)
		fmt.Fprintf(os.Stderr, "error: %s\n", message.Ref().String())
		message.Drop(c)
		status = 1
	}
	result.Drop(c)
	if c.Failed() {
		panic("uncaught Rust panic from renderer CLI")
	}
	renderer.RustExit(c, int32(status))
}
