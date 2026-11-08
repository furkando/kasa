package ui

import (
	"strings"
	"testing"

	"github.com/furkando/kasa/internal/config"
)

func newCollapseModel(t *testing.T) (Model, *config.Config, string) {
	t.Helper()
	m, cfg, path := newTestModel(t)
	cfg.Assets.Deposits = append(cfg.Assets.Deposits, config.Deposit{
		Name: "İkinci", Principal: 500000, GrossRate: 0.38, Stopaj: 0.175,
		Start: cfg.Assets.Deposits[0].Start, Maturity: cfg.Assets.Deposits[0].Maturity,
	})
	m.recompute()
	return m, cfg, path
}

func TestCollapseGroup(t *testing.T) {
	m, cfg, path := newCollapseModel(t)
	step := stepper(t, &m)

	selectRow(t, &m, step, "vadeli:İş Vadeli")
	step(key("c"))

	out := m.viewTable(110) // wide enough for the MİKTAR column
	if strings.Contains(out, "İş Vadeli") || strings.Contains(out, "İkinci") {
		t.Fatalf("collapsed rows still rendered:\n%s", out)
	}
	if !strings.Contains(out, "VADELİ") || !strings.Contains(out, "2 kalem") {
		t.Fatalf("collapsed header missing:\n%s", out)
	}
	saved, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.IsCollapsed("vadeli") {
		t.Fatal("collapse not persisted")
	}

	// row actions must not reach rows that are not on screen
	before := len(cfg.Assets.Deposits)
	step(key("x"))
	if m.confirmDel != "" {
		t.Fatalf("delete armed on collapsed row %q", m.confirmDel)
	}
	step(key("e"))
	if m.mode != modeDash || len(cfg.Assets.Deposits) != before {
		t.Fatal("edit opened on collapsed group")
	}

	step(key("enter"))
	if m.mode != modeDash || cfg.IsCollapsed("vadeli") {
		t.Fatal("enter should expand a collapsed group")
	}
	if out := m.viewTable(96); !strings.Contains(out, "İkinci") {
		t.Fatalf("rows not restored after expand:\n%s", out)
	}
}

func TestCollapsedGroupIsOneLineForNavigation(t *testing.T) {
	m, cfg, _ := newCollapseModel(t)
	step := stepper(t, &m)

	cfg.ToggleCollapsed("vadeli")
	m.sel = 0
	if m.sum.Rows[0].Category != "altın" {
		t.Fatalf("expected gold first, got %s", m.sum.Rows[0].Category)
	}
	step(key("down"))
	if !m.selCollapsed("vadeli") {
		t.Fatal("down should land on the collapsed group")
	}
	at := m.sel
	step(key("down")) // nothing after the group
	if m.sel != at {
		t.Fatalf("down walked inside collapsed group: %d → %d", at, m.sel)
	}
	step(key("up"))
	if m.sum.Rows[m.sel].Category != "altın" {
		t.Fatalf("up did not leave the group: %s", m.sum.Rows[m.sel].Key)
	}
}

func TestCollapseAll(t *testing.T) {
	m, cfg, _ := newCollapseModel(t)
	step := stepper(t, &m)

	step(key("C"))
	if !cfg.IsCollapsed("altın") || !cfg.IsCollapsed("vadeli") {
		t.Fatalf("collapse all: %v", cfg.Collapsed)
	}
	step(key("C"))
	if len(cfg.Collapsed) != 0 {
		t.Fatalf("expand all: %v", cfg.Collapsed)
	}
}

func TestFlatViewIgnoresCollapse(t *testing.T) {
	m, cfg, _ := newCollapseModel(t)
	step := stepper(t, &m)

	cfg.ToggleCollapsed("vadeli")
	step(key("g"))
	if out := m.viewTable(96); !strings.Contains(out, "İkinci") {
		t.Fatalf("flat view hid collapsed rows:\n%s", out)
	}
}

func TestCollapseHiddenSection(t *testing.T) {
	for _, flat := range []bool{false, true} {
		m, cfg, _ := newCollapseModel(t)
		step := stepper(t, &m)
		if flat {
			step(key("g"))
		}
		cfg.ToggleHidden("gold")
		m.recompute()

		selectRow(t, &m, step, "gold")
		step(key("c"))

		out := m.viewTable(110)
		if strings.Contains(out, "Altın") {
			t.Fatalf("flat=%v: hidden row still rendered:\n%s", flat, out)
		}
		if !strings.Contains(out, "GİZLİ") || !strings.Contains(out, "1 kalem") {
			t.Fatalf("flat=%v: hidden header missing:\n%s", flat, out)
		}
		if !m.selCollapsed(hiddenGroup) {
			t.Fatalf("flat=%v: cursor should be on the hidden section", flat)
		}

		step(key("x"))
		if m.confirmDel != "" {
			t.Fatalf("flat=%v: delete armed on collapsed row", flat)
		}
		step(key("h"))
		if !cfg.IsHidden("gold") {
			t.Fatalf("flat=%v: h reached a collapsed row", flat)
		}

		step(key("enter"))
		if m.mode != modeDash || cfg.IsCollapsed(hiddenGroup) {
			t.Fatalf("flat=%v: enter should expand the hidden section", flat)
		}
		if out := m.viewTable(110); !strings.Contains(out, "Altın") {
			t.Fatalf("flat=%v: hidden row not restored:\n%s", flat, out)
		}
	}
}

func TestCollapseAllIncludesHiddenSection(t *testing.T) {
	m, cfg, _ := newCollapseModel(t)
	step := stepper(t, &m)

	cfg.ToggleHidden("gold")
	m.recompute()
	step(key("C"))
	if !cfg.IsCollapsed(hiddenGroup) || !cfg.IsCollapsed("vadeli") {
		t.Fatalf("collapse all: %v", cfg.Collapsed)
	}
}

func TestTruncateMiddleKeepsDistinctEnds(t *testing.T) {
	a, b := truncateMiddle("Daire (merkez - kuzey)", 14), truncateMiddle("Daire (merkez - güney)", 14)
	if a == b || len([]rune(a)) != 14 || len([]rune(b)) != 14 {
		t.Fatalf("got %q / %q", a, b)
	}
	if got := truncateMiddle("kt", 14); got != "kt" {
		t.Fatalf("short label changed: %q", got)
	}
}

func TestLoansSplitIntoAlacakAndBorc(t *testing.T) {
	m, cfg, _ := newCollapseModel(t)
	cfg.Assets.Loans = []config.Loan{
		{Name: "ali", Currency: "USD", Amount: 500, Kind: "alacak"},
		{Name: "veli", Currency: "USD", Amount: 900, Kind: "borc"},
	}
	m.recompute()
	got := map[string]string{}
	for _, r := range m.sum.Rows {
		got[r.Key] = r.Category
	}
	if got["loan:ali"] != "alacak" || got["loan:veli"] != "borç" {
		t.Fatalf("categories: %v", got)
	}
}
