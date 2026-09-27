package mir

import (
	"strings"
	"testing"
)

func TestCallerLocationMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tracked  bool
		location CallLocation
		want     string
	}{
		{"inherited", true, CallLocation{Inherited: true}, "caller"},
		{"static", false, CallLocation{Allocation: 17, Offset: 8}, "(oxideGlobals.Get(17)+8)"},
		// An inlined function can stop track_caller propagation. The compiler's
		// choice takes precedence over the containing function's attribute.
		{"inlined static", true, CallLocation{Allocation: 17}, "(oxideGlobals.Get(17)+0)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &generator{
				f:           &Function{TrackCaller: tc.tracked, CallLocations: map[int]CallLocation{3: tc.location}},
				knownAllocs: map[uint64]bool{17: true},
			}
			if got := g.callerLocation(3); got != tc.want {
				t.Fatalf("caller location = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestCallerLocationRejectsMissingMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Function
		want string
	}{
		{"missing", Function{}, "missing compiler caller location"},
		{"invalid inheritance", Function{CallLocations: map[int]CallLocation{0: {Inherited: true}}}, "without track_caller"},
		{"missing allocation", Function{CallLocations: map[int]CallLocation{0: {Allocation: 17}}}, "missing caller location allocation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				err, ok := recover().(generationError)
				if !ok || !strings.Contains(string(err), tc.want) {
					t.Fatalf("expected %q error, got %q", tc.want, err)
				}
			}()
			g := &generator{f: &tc.f}
			g.callerLocation(0)
		})
	}
}
