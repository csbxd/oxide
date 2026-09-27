package main

import (
	"os"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
	renderer "oxide-renderer-conformance"
)

func main() {
	c := oxide.NewContext()
	status := renderer.Fixture_CliMain(c)
	if c.Failed() {
		panic("uncaught Rust panic from renderer CLI")
	}
	if err := c.Close(); err != nil {
		panic(err)
	}
	os.Exit(int(status))
}
