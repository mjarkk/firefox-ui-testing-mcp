package src

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderAria(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"tree": []map[string]any{
			{"r": "navigation", "n": "Main", "c": []map[string]any{
				{"r": "list", "c": []map[string]any{
					{"r": "listitem", "c": []map[string]any{{"r": "link", "n": "Home", "id": "e1", "href": "/"}}},
				}},
			}},
			{"t": "Email"},
			{"r": "textbox", "n": "Email", "id": "e2", "st": []string{"required"}},
			{"r": "status"},
			{"r": "table", "c": []map[string]any{
				{"r": "row", "c": []map[string]any{{"r": "cell", "n": "a"}, {"r": "cell", "n": "b"}}},
			}},
		},
		"next": 3,
	})
	snap, err := parseAriaSnapshot(string(raw))
	if err != nil {
		t.Fatal(err)
	}

	want := `navigation "Main"
  list
    [e1] link "Home" -> /
[e2] textbox "Email" required
table
  row: a | b
`
	if got := renderAria(snap, "", 0); got != want {
		t.Errorf("full dump:\n%s\nwant:\n%s", got, want)
	}

	if got := renderAria(snap, "home", 0); !strings.Contains(got, "navigation") || strings.Contains(got, "textbox") {
		t.Errorf("text query kept the wrong nodes:\n%s", got)
	}
	if got := renderAria(snap, "e2", 0); got != "[e2] textbox \"Email\" required\n" {
		t.Errorf("id query:\n%s", got)
	}

	if got := renderAria(snap, "sidebar", 0); !strings.Contains(got, "nothing matched") || !strings.Contains(got, "roles on this page: cell, link, list, navigation, row, table, textbox") {
		t.Errorf("no match:\n%s", got)
	}
	if got := renderAria(snap, "e9", 0); got != "(no element with id \"e9\" on the page)\n" {
		t.Errorf("no id match:\n%s", got)
	}
}
