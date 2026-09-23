package src

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

//go:embed aria.js
var ariaJS string

const lookupJS = `(id) => {
	const ref = window.__bmcp && window.__bmcp.els.get(id);
	const el = ref && ref.deref();
	return el && el.isConnected ? el : null;
}`

type ariaNode struct {
	Role        string      `json:"r"`
	Name        string      `json:"n"`
	ID          string      `json:"id"`
	Value       string      `json:"v"`
	Placeholder string      `json:"ph"`
	States      []string    `json:"st"`
	Level       int         `json:"lvl"`
	Href        string      `json:"href"`
	TestID      string      `json:"tid"`
	Text        string      `json:"t"`
	Children    []*ariaNode `json:"c"`
}

type ariaSnapshot struct {
	Tree    []*ariaNode `json:"tree"`
	Next    int         `json:"next"`
	URL     string      `json:"url"`
	Title   string      `json:"title"`
	ScrollX int         `json:"sx"`
	ScrollY int         `json:"sy"`
	ScrollW int         `json:"sw"`
	ScrollH int         `json:"sh"`
}

func parseAriaSnapshot(raw any) (*ariaSnapshot, error) {
	str, ok := raw.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected aria dump result %T", raw)
	}
	var snap ariaSnapshot
	if err := json.Unmarshal([]byte(str), &snap); err != nil {
		return nil, err
	}
	snap.Tree = compactNodes(snap.Tree)
	return &snap, nil
}

// compactNodes removes wrappers that add lines without adding information.
func compactNodes(nodes []*ariaNode) []*ariaNode {
	out := make([]*ariaNode, 0, len(nodes))
	for i, n := range nodes {
		n.Children = compactNodes(n.Children)
		if n.isEmpty() {
			continue
		}
		// Label text right next to the control it names is already in the control's name.
		if n.Role == "" && ((i > 0 && nodes[i-1].Name == n.Text) || (i+1 < len(nodes) && nodes[i+1].Name == n.Text)) {
			continue
		}
		if n.Name != "" {
			// Text that is already part of the name adds nothing.
			kept := n.Children[:0]
			for _, c := range n.Children {
				if c.Role != "" || !strings.Contains(n.Name, c.Text) {
					kept = append(kept, c)
				}
			}
			n.Children = kept
		}
		wrapper := n.Role == "listitem" || n.Role == "cell"
		if wrapper && n.ID == "" && n.Name == "" && len(n.States) == 0 && n.TestID == "" && len(n.Children) == 1 {
			out = append(out, n.Children[0])
			continue
		}
		out = append(out, n)
	}
	return out
}

func (n *ariaNode) isEmpty() bool {
	if n.Role == "" {
		return n.Text == ""
	}
	return n.ID == "" && n.Name == "" && n.Value == "" && len(n.States) == 0 && n.TestID == "" &&
		len(n.Children) == 0 && n.Role != "separator" && n.Role != "iframe"
}

func (n *ariaNode) line() string {
	if n.Role == "" {
		return strconv.Quote(n.Text)
	}

	var b strings.Builder
	if n.ID != "" {
		b.WriteString("[" + n.ID + "] ")
	}
	b.WriteString(n.Role)
	if n.Level > 0 {
		fmt.Fprintf(&b, "[%d]", n.Level)
	}
	if n.Name != "" {
		b.WriteString(" " + strconv.Quote(n.Name))
	}
	if n.Value != "" {
		b.WriteString(" value=" + strconv.Quote(n.Value))
	}
	if n.Placeholder != "" {
		b.WriteString(" placeholder=" + strconv.Quote(n.Placeholder))
	}
	if n.Href != "" {
		b.WriteString(" -> " + n.Href)
	}
	if n.TestID != "" {
		b.WriteString(" testid=" + n.TestID)
	}
	for _, s := range n.States {
		b.WriteString(" " + s)
	}
	return b.String()
}

// tableRowLine renders a row of plain cells on a single line.
func (n *ariaNode) tableRowLine() (string, bool) {
	if n.Role != "row" || n.ID != "" || len(n.Children) == 0 {
		return "", false
	}
	cells := make([]string, 0, len(n.Children))
	for _, c := range n.Children {
		isCell := c.Role == "cell" || c.Role == "columnheader" || c.Role == "rowheader"
		if !isCell || c.ID != "" || len(c.Children) > 0 || len(c.States) > 0 {
			return "", false
		}
		cells = append(cells, c.Name)
	}
	return "row: " + strings.Join(cells, " | "), true
}

func renderNodes(b *strings.Builder, nodes []*ariaNode, depth int) {
	for _, n := range nodes {
		b.WriteString(strings.Repeat("  ", depth))
		if row, ok := n.tableRowLine(); ok {
			b.WriteString(row + "\n")
			continue
		}
		b.WriteString(n.line() + "\n")
		renderNodes(b, n.Children, depth+1)
	}
}

var idQueryRe = regexp.MustCompile(`^\[?e\d+\]?$`)

// filterNodes keeps nodes matching the query (with their full subtree) and
// the ancestors needed to place them in context.
func filterNodes(nodes []*ariaNode, query string) []*ariaNode {
	query = strings.ToLower(strings.TrimSpace(query))
	idQuery := ""
	if idQueryRe.MatchString(query) {
		idQuery = strings.Trim(query, "[]")
	}

	var walk func([]*ariaNode) []*ariaNode
	walk = func(nodes []*ariaNode) []*ariaNode {
		var out []*ariaNode
		for _, n := range nodes {
			matched := false
			if idQuery != "" {
				matched = n.ID == idQuery
			} else {
				matched = strings.Contains(strings.ToLower(n.line()), query)
			}
			if matched {
				out = append(out, n)
				continue
			}
			if kept := walk(n.Children); len(kept) > 0 {
				cp := *n
				cp.Children = kept
				out = append(out, &cp)
			}
		}
		return out
	}
	return walk(nodes)
}

func renderAria(snap *ariaSnapshot, query string, maxChars int) string {
	nodes := snap.Tree
	if query != "" {
		nodes = filterNodes(nodes, query)
		if len(nodes) == 0 {
			return noMatchMessage(snap.Tree, query)
		}
	}

	var b strings.Builder
	renderNodes(&b, nodes, 0)
	out := b.String()
	if out == "" {
		return "(page has no visible content)\n"
	}
	if maxChars > 0 && len(out) > maxChars {
		cut := strings.LastIndexByte(out[:maxChars], '\n') + 1
		out = out[:cut] + fmt.Sprintf("… truncated %d more characters, use an aria query to narrow the dump down\n", len(out)-cut)
	}
	return out
}

// noMatchMessage explains an empty query result and lists the roles on the
// page so the next query can target something that exists.
func noMatchMessage(nodes []*ariaNode, query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if idQueryRe.MatchString(q) {
		return fmt.Sprintf("(no element with id %q on the page)\n", strings.Trim(q, "[]"))
	}

	var roles []string
	var walk func([]*ariaNode)
	walk = func(nodes []*ariaNode) {
		for _, n := range nodes {
			if n.Role != "" && !slices.Contains(roles, n.Role) {
				roles = append(roles, n.Role)
			}
			walk(n.Children)
		}
	}
	walk(nodes)
	slices.Sort(roles)

	msg := fmt.Sprintf("(nothing matched query %q, the query matches the text of dump lines (role, name, value, link target) case insensitive", query)
	if len(roles) > 0 {
		msg += ", roles on this page: " + strings.Join(roles, ", ")
	}
	return msg + ")\n"
}
