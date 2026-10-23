package library

// Read native declarations without importing or executing the harness. Parsing
// an AST avoids treating comments, descriptions or dynamic expressions as names.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/js"
)

type promptJSVisitor struct{ visit func(js.INode) bool }

func (v promptJSVisitor) Enter(n js.INode) js.IVisitor {
	if v.visit(n) {
		return v
	}
	return nil
}
func (v promptJSVisitor) Exit(js.INode) {}
func promptJSString(n js.INode) string  { var s strings.Builder; n.JS(&s); return s.String() }
func promptJSLiteral(e js.IExpr) (string, bool) {
	lit, ok := e.(*js.LiteralExpr)
	if !ok {
		return "", false
	}
	var b strings.Builder
	if lit.JSON(&b) != nil {
		return "", false
	}
	var s string
	err := json.Unmarshal([]byte(b.String()), &s)
	return s, err == nil
}

func promptJSUsesText(n js.INode) bool {
	found := false
	js.Walk(promptJSVisitor{func(n js.INode) bool {
		if v, ok := n.(*js.Var); ok && string(v.Name()) == "text" {
			found = true
		}
		return !found
	}}, n)
	return found
}

// A familiar method and catalog do not prove a new dispatch idiom is covered.
// Conditions consuming the input must use the same literal comparisons that
// the inventory below reads, or the native extension-command predicate covered
// by get_commands. Unknown conditions cannot silently add unreserved names.
func piKnownDispatchCondition(e js.IExpr) bool {
	if !promptJSUsesText(e) {
		return true
	}
	switch x := e.(type) {
	case *js.GroupExpr:
		return piKnownDispatchCondition(x.X)
	case *js.Var:
		return string(x.Name()) == "text"
	case *js.UnaryExpr:
		return x.Op == js.NotToken && piKnownDispatchCondition(x.X)
	case *js.BinaryExpr:
		if x.Op == js.OrToken || x.Op == js.AndToken {
			return piKnownDispatchCondition(x.X) && piKnownDispatchCondition(x.Y)
		}
		if x.Op == js.EqEqEqToken && promptJSString(x.X) == "text" {
			_, ok := promptJSLiteral(x.Y)
			return ok
		}
	case *js.CallExpr:
		if len(x.Args.List) != 1 || x.Args.List[0].Rest {
			return false
		}
		switch promptJSString(x.X) {
		case "text.startsWith":
			_, ok := promptJSLiteral(x.Args.List[0].Value)
			return ok
		case "this.isExtensionCommand":
			return promptJSString(x.Args.List[0].Value) == "text"
		}
	}
	return false
}

func piInstalledPackage(t PromptTarget, bin string) (string, error) {
	var candidates []string
	// A simple native shim can point outside its own npm directory (for
	// example an isolated verification context). Read that original entry;
	// never execute a shim to discover its source or rewrite its program.
	if strings.EqualFold(filepath.Ext(bin), ".cmd") {
		if b, err := os.ReadFile(bin); err == nil && len(b) <= 64<<10 {
			for _, match := range regexp.MustCompile(`"([^"\r\n]+\.js)"`).FindAllStringSubmatch(string(b), -1) {
				entry := match[1]
				if !filepath.IsAbs(entry) || strings.ContainsAny(entry, "%!") {
					continue
				}
				for d := filepath.Dir(entry); filepath.Dir(d) != d; d = filepath.Dir(d) {
					candidates = append(candidates, d)
				}
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(t.dir, "install", "current-version")); err == nil {
		v := strings.TrimSpace(string(b))
		if !filepath.IsLocal(v) || strings.ContainsAny(v, `/\`) {
			return "", fmt.Errorf("invalid Pi release selector")
		}
		for _, scope := range []string{"@earendil-works", "@mariozechner"} {
			candidates = append(candidates, filepath.Join(t.dir, "install/releases", v, "node_modules", scope, "pi-coding-agent"))
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	for d := filepath.Dir(bin); ; d = filepath.Dir(d) {
		candidates = append(candidates, d)
		for _, scope := range []string{"@earendil-works", "@mariozechner"} {
			candidates = append(candidates, filepath.Join(d, "node_modules", scope, "pi-coding-agent"))
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	for _, root := range candidates {
		b, err := os.ReadFile(filepath.Join(root, "package.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		var p struct {
			Name string
			Bin  json.RawMessage
		}
		if json.Unmarshal(b, &p) != nil {
			return "", fmt.Errorf("invalid Pi package manifest: %s", root)
		}
		if !slices.Contains([]string{"@earendil-works/pi-coding-agent", "@mariozechner/pi-coding-agent"}, p.Name) {
			continue
		}
		var entry string
		if json.Unmarshal(p.Bin, &entry) != nil {
			var bins map[string]string
			if json.Unmarshal(p.Bin, &bins) != nil {
				return "", fmt.Errorf("invalid Pi executable declaration: %s", root)
			}
			entry = bins["pi"]
		}
		if !filepath.IsLocal(entry) {
			return "", fmt.Errorf("invalid Pi executable declaration: %s", root)
		}
		if _, err := os.Stat(filepath.Join(root, entry)); err != nil {
			return "", err
		}
		return root, nil
	}
	return "", &promptRuntimeError{"Pi built-in command definitions could not be verified", bin}
}

func piInstalledBuiltins(root string) ([]string, error) {
	bad := func() ([]string, error) {
		return nil, &promptRuntimeError{"Pi built-in command definitions could not be verified", root}
	}
	read := func(p string) (*js.AST, error) {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			return nil, err
		}
		if len(b) > 4<<20 {
			return nil, fmt.Errorf("native command definitions exceed 4 MiB")
		}
		return js.Parse(parse.NewInputBytes(b), js.Options{})
	}
	ast, err := read("dist/core/slash-commands.js")
	if err != nil {
		return bad()
	}
	var names []string
	found, invalid := false, false
	js.Walk(promptJSVisitor{func(n js.INode) bool {
		decl, ok := n.(*js.VarDecl)
		if !ok {
			return true
		}
		for _, binding := range decl.List {
			if promptJSString(binding.Binding) != "BUILTIN_SLASH_COMMANDS" {
				continue
			}
			if found {
				invalid = true
			}
			found = true
			array, ok := binding.Default.(*js.ArrayExpr)
			if !ok {
				invalid = true
				continue
			}
			for _, el := range array.List {
				obj, ok := el.Value.(*js.ObjectExpr)
				if !ok || el.Spread {
					invalid = true
					continue
				}
				name := ""
				for _, prop := range obj.List {
					if prop.Spread || prop.Name == nil || prop.Name.Computed != nil {
						invalid = true
						continue
					}
					if !prop.Name.IsIdent([]byte("name")) {
						continue
					}
					s, ok := promptJSLiteral(prop.Value)
					if !ok || s == "" || name != "" {
						invalid = true
					}
					name = s
				}
				if name == "" {
					invalid = true
				}
				names = append(names, name)
			}
		}
		return true
	}}, ast)
	if !found || invalid || len(names) == 0 {
		return bad()
	}
	ast, err = read("dist/modes/interactive/interactive-mode.js")
	if err != nil {
		return bad()
	}
	found, handoff := false, false
	js.Walk(promptJSVisitor{func(n js.INode) bool {
		method, ok := n.(*js.MethodDecl)
		if !ok || !method.Name.IsIdent([]byte("setupEditorSubmitHandler")) {
			return true
		}
		if found {
			invalid = true
		}
		found = true
		js.Walk(promptJSVisitor{func(n js.INode) bool {
			switch x := n.(type) {
			case *js.IfStmt:
				invalid = invalid || !piKnownDispatchCondition(x.Cond)
			case *js.CondExpr:
				invalid = invalid || !piKnownDispatchCondition(x.Cond)
			case *js.SwitchStmt:
				invalid = invalid || promptJSUsesText(x.Init)
			case *js.BinaryExpr:
				if x.Op == js.EqEqEqToken && promptJSString(x.X) == "text" {
					s, ok := promptJSLiteral(x.Y)
					if !ok {
						invalid = true
					} else if strings.HasPrefix(s, "/") {
						names = append(names, strings.TrimSpace(strings.TrimPrefix(s, "/")))
					}
				}
			case *js.CallExpr:
				s := promptJSString(x.X)
				if s == "this.session.prompt" {
					handoff = true
				}
				if s == "text.startsWith" {
					if len(x.Args.List) != 1 {
						invalid = true
						break
					}
					v, ok := promptJSLiteral(x.Args.List[0].Value)
					if !ok {
						invalid = true
					} else if strings.HasPrefix(v, "/") {
						names = append(names, strings.TrimSpace(v[1:]))
					}
				}
			}
			return true
		}}, &method.Body)
		return false
	}}, ast)
	if !found || !handoff || invalid {
		return bad()
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
