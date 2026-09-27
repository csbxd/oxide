package mir

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var asmTemplatePiece = regexp.MustCompile(`String\("(?:[^"\\]|\\.)*"\)|Placeholder \{ operand_idx: [0-9]+, modifier: (?:None|Some\('[^']'\)), span: [^}]* \}`)

// rustc_public currently exports the template's Debug representation. Accept
// only a fully parsed sequence whose assembled text consists of comments.
// Finding a comment somewhere in an instruction is not evidence of a no-op.
func commentOnlyAsm(template string) bool {
	if !strings.HasPrefix(template, "[") || !strings.HasSuffix(template, "]") {
		return false
	}
	s := template[1 : len(template)-1]
	var source strings.Builder
	cursor := 0
	for _, span := range asmTemplatePiece.FindAllStringIndex(s, -1) {
		if strings.Trim(s[cursor:span[0]], " ,\n\t") != "" {
			return false
		}
		piece := s[span[0]:span[1]]
		if strings.HasPrefix(piece, "String(") {
			literal, err := strconv.Unquote(piece[7 : len(piece)-1])
			if err != nil {
				return false
			}
			source.WriteString(literal)
		} else {
			source.WriteString("register")
		}
		cursor = span[1]
	}
	if strings.Trim(s[cursor:], " ,\n\t") != "" {
		return false
	}
	s = strings.TrimSpace(source.String())
	for strings.HasPrefix(s, "/*") {
		end := strings.Index(s[2:], "*/")
		if end < 0 {
			return false
		}
		s = strings.TrimSpace(s[end+4:])
	}
	return s == ""
}

func (g *generator) inlineAsm(raw json.RawMessage) {
	if g.x86CPUIDAsm(raw) {
		return
	}
	x := decode[struct {
		Template    string `json:"template"`
		Destination *int   `json:"destination"`
		Options     string `json:"options"`
		Operands    []struct {
			Input  json.RawMessage `json:"in_value"`
			Output *Place          `json:"out_place"`
			Raw    string          `json:"raw_rpr"`
		} `json:"operands"`
	}](raw)
	if !commentOnlyAsm(x.Template) || x.Destination == nil || x.Options != "" {
		g.fail("unsupported inline asm template or options")
	}
	for _, operand := range x.Operands {
		kind, data := variant(operand.Input)
		if !strings.HasPrefix(operand.Raw, "InOut {") || (kind != "Copy" && kind != "Move") || operand.Output == nil || !reflect.DeepEqual(decode[Place](data), *operand.Output) {
			g.fail("unsupported inline asm operand; expected identity inout")
		}
	}
	g.line("oxide.CompilerBarrier(); goto bb%d", *x.Destination)
}
