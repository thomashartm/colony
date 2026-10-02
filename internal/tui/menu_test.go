package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
)

func githubRow(id string) member.Row {
	r := row(id, true)
	r.RemoteURL, r.Base = "git@github.com:acme/api.git", "main"
	return r
}

func TestMemberLinksOrderAndFiltering(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m.crews = []crew.Crew{{ID: "fx", Title: "FX", URL: "https://example.com/fx", Color: "blue"}, {ID: "bad", Title: "Bad", URL: "-flag", Color: "red"}}
	r := githubRow("alpha")
	r.Crew = "fx"
	var labels []string
	for _, l := range m.memberLinks(r) {
		labels = append(labels, l.label+"="+l.url)
	}
	want := "Branch feat/alpha=https://github.com/acme/api/tree/feat/alpha|Compare main...feat/alpha=https://github.com/acme/api/compare/main...feat/alpha|Crew FX=https://example.com/fx"
	if got := strings.Join(labels, "|"); got != want {
		t.Fatalf("links:\n%s\nwant\n%s", got, want)
	}
	r.RemoteURL, r.Crew = "/tmp/origin.git", "bad"
	if links := m.memberLinks(r); len(links) != 0 {
		t.Fatalf("non-GitHub remote and non-web crew URL must give no links: %+v", links)
	}
}

func TestBrowserMenuOpensSelectedLink(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	var opened []string
	m.openURL = func(u string) error { opened = append(opened, u); return nil }
	m = update(m, key("b"))
	if m.menu == nil || !strings.Contains(ansi.Strip(m.menuView(20)), "> Branch feat/alpha") {
		t.Fatalf("menu not shown: %+v", m.menu)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.menu != nil || cmd == nil {
		t.Fatal("enter must close the menu and return the open command")
	}
	m = update(m, cmd())
	if len(opened) != 1 || opened[0] != "https://github.com/acme/api/compare/main...feat/alpha" {
		t.Fatalf("opened %v", opened)
	}
}

func TestBrowserMenuWithoutLinksAndOpenFailure(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, snapshot{rows: []member.Row{row("plain", true)}})
	m = update(m, key("b"))
	if m.menu != nil || m.message != "No links for this member" {
		t.Fatalf("menu=%v message=%q", m.menu, m.message)
	}
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	m.openURL = func(string) error { return errors.New("xdg-open: not found") }
	m = update(m, key("b"))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = update(m, cmd())
	if !strings.Contains(m.message, "xdg-open: not found") {
		t.Fatalf("message %q", m.message)
	}
}

func TestBrowserMenuEscapeCancels(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	m = update(m, key("b"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.menu != nil {
		t.Fatal("esc must close the menu")
	}
}

func TestDetailBranchAndCompareAreHyperlinks(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 160, Height: 40})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	view := m.detail.View()
	for _, want := range []string{"\x1b]8;;https://github.com/acme/api/tree/feat/alpha\x1b\\", "\x1b]8;;https://github.com/acme/api/compare/main...feat/alpha\x1b\\"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail lacks %q:\n%q", want, view)
		}
	}
}

func TestBrowserMenuMouseClickOpensItem(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	var opened []string
	m.openURL = func(u string) error { opened = append(opened, u); return nil }
	m = update(m, key("b"))
	// Second item: header row, top border, heading gap, title, blank, first item.
	y := 2 + m.panelHeadingGap() + 2 + 1
	next, cmd := m.Update(tea.MouseMsg{X: m.listWidth() + 5, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if next.(Model).menu != nil || cmd == nil {
		t.Fatal("click must run the item")
	}
	cmd()
	if len(opened) != 1 || !strings.Contains(opened[0], "/compare/") {
		t.Fatalf("opened %v", opened)
	}
}

func TestBrowserMenuFooterAndAction(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 160, Height: 30})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	if !strings.Contains(m.footer(), "b browser") {
		t.Fatalf("list footer lacks b:\n%s", m.footer())
	}
	found := false
	for _, a := range m.actions() {
		found = found || (a.key == "b" && a.label == "Open in browser (b)")
	}
	if !found {
		t.Fatal("actions lack Open in browser (b)")
	}
	m = update(m, key("b"))
	if !strings.Contains(m.footer(), "[Menu]") || strings.Contains(m.footer(), m.navigationBar()) {
		t.Fatalf("menu footer:\n%s", m.footer())
	}
}

func TestBrowserHintOnlyWhenItFits(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(m, snapshot{rows: []member.Row{githubRow("alpha")}})
	if f := m.footer(); strings.Contains(f, "b browser") || !strings.Contains(f, "Nav:") {
		t.Fatalf("80 columns must keep the full footer without the optional hint:\n%s", f)
	}
}
