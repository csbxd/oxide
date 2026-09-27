package oxide

import "testing"

func TestCheckedIntegerOperations(t *testing.T) {
	if Add[int32](3, 4) != 7 || Sub[int32](7, 4) != 3 || Mul[int32](3, 4) != 12 || Div[int32](12, 3) != 4 || Rem[int32](13, 5) != 3 {
		t.Fatal("basic checked arithmetic is wrong")
	}
	for name, f := range map[string]func(){
		"add":      func() { Add[int8](127, 1) },
		"sub":      func() { Sub[int8](-128, 1) },
		"mul":      func() { Mul[int8](64, 2) },
		"div-zero": func() { Div[int32](1, 0) },
		"rem-zero": func() { Rem[int32](1, 0) },
	} {
		f := f
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected Rust panic")
				}
			}()
			f()
		})
	}
}
