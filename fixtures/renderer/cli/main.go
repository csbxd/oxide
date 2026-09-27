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
	if result.Variant() == "Err" {
		message := result.Field("0").Display(c)
		fmt.Fprintf(os.Stderr, "error: %s\n", message.String())
		message.Drop(c)
		status = 1
	}
	result.Drop(c)
	if c.Failed() {
		panic("uncaught Rust panic from renderer CLI")
	}
	renderer.RustExit(c, int32(status))
}
