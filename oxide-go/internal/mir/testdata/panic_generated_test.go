package fixture

import (
	"fmt"
	"os"
	"strings"
	"testing"

	oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func TestPanicResultsAndFrames(t *testing.T) {
	expected, err := os.ReadFile("expected.stdout")
	if err != nil {
		t.Fatal(err)
	}
	ctx := oxide.NewContext()
	defer ctx.Close()
	for _, line := range strings.Split(strings.TrimSpace(string(expected)), "\n") {
		var caseID, seed, want uint64
		if _, err := fmt.Sscanf(line, "%d %d %d", &caseID, &seed, &want); err != nil {
			t.Fatal(err)
		}
		t.Run(fmt.Sprintf("case_%d/seed_%d", caseID, seed), func(t *testing.T) {
			mark := ctx.Mark()
			requireNoGoAllocations(t, 100, func() {
				if got := Conformance(ctx, caseID, seed); got != want {
					t.Fatalf("translated panic/drop result = %d, native Rust result = %d", got, want)
				}
				if ctx.Failed() || ctx.Mark() != mark {
					t.Fatal("uncaught panic or leaked Rust automatic storage")
				}
			})
		})
	}
}
