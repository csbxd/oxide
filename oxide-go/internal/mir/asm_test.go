package mir

import "testing"

func TestCommentOnlyAsm(t *testing.T) {
	for _, template := range []string{
		`[]`,
		`[String("/* no instruction */")]`,
		`[String("/*"), Placeholder { operand_idx: 0, modifier: None, span: /tmp/probe.rs:1:2: 1:4 (#0) }, String("*/")]`,
		`[String("/*"), Placeholder { operand_idx: 0, modifier: Some('v'), span: /tmp/probe.rs:1:2: 1:4 (#0) }, String("*/ /* more */")]`,
	} {
		if !commentOnlyAsm(template) {
			t.Errorf("rejected comment template %s", template)
		}
	}
	for _, template := range []string{
		`[String("/* comment */ mov x0, x1")]`,
		`[String("mov x0, x1 /* comment */")]`,
		`[String("/* comment */"), Placeholder { operand_idx: 0, modifier: None, span: probe.rs:1:1 (#0) }]`,
		`[String("/*"), String("*/\nsvc 0\n/*"), String("*/")]`,
		`[String("/* unclosed")]`,
		`[String("/* comment */"), Unknown("instruction")]`,
		`[String("/* comment */"), String("*/")]`,
		`String("/* comment */")`,
	} {
		if commentOnlyAsm(template) {
			t.Errorf("accepted non-comment template %s", template)
		}
	}
}
