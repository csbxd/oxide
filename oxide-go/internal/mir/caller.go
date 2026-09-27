package mir

import "fmt"

// rustc resolves inlined scopes when choosing whether a call inherits the
// hidden location or uses a static Location. TrackCaller alone is insufficient.
func (g *generator) callerLocation(block int) string {
	location, ok := g.f.CallLocations[block]
	if !ok {
		g.fail("missing compiler caller location for block %d", block)
	}
	if location.Inherited {
		if !g.f.TrackCaller {
			g.fail("block %d inherits a caller location without track_caller", block)
		}
		return "caller"
	}
	if !g.knownAllocs[location.Allocation] {
		g.fail("block %d refers to missing caller location allocation %d", block, location.Allocation)
	}
	return fmt.Sprintf("(oxideGlobals.Get(%d)+%d)", location.Allocation, location.Offset)
}
