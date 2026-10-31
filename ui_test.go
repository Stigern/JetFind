package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"jetfind/scan"
	"jetfind/store"
)

const testMAC = "30:52:53:00:1a:2b"

// newTestUI builds the real window content on the test driver, in demo mode
// so refreshes stay off the network.
func newTestUI(t *testing.T) *ui {
	a := test.NewApp()
	a.Settings().SetTheme(darkTheme{})
	u := &ui{app: a, demo: true, store: store.Open(memPrefs{}),
		byKey: map[string]*deviceCard{}, holders: map[*deviceCard]fyne.CanvasObject{}}
	test.NewWindow(u.build())
	return u
}

func testCard(t *testing.T) (*deviceCard, fyne.Canvas, *[]string) {
	a := test.NewApp()
	a.Settings().SetTheme(darkTheme{})
	names := &[]string{}
	c := newDeviceCard("k", scan.Device{Name: "jetkvm", IP: "192.168.1.21"}, deviceStatus{}, cardActions{
		rename: func(_ *deviceCard, n string) { *names = append(*names, n) },
	})
	w := test.NewWindow(c)
	w.Resize(fyne.NewSize(400, 200))
	return c, w.Canvas(), names
}

func TestPencilReturnsAfterRenameWhileHovered(t *testing.T) {
	for name, end := range map[string]func(*deviceCard){
		"enter":  func(c *deviceCard) { c.editor.OnSubmitted("x") },
		"escape": func(c *deviceCard) { c.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) },
	} {
		c, _, _ := testCard(t)
		c.MouseIn(nil)
		c.startRename()
		end(c)
		if !c.pencil.Visible() {
			t.Errorf("%s: pencil hidden while the card is hovered", name)
		}
	}
}

func TestPencilStaysHiddenAfterClickingAway(t *testing.T) {
	c, cv, _ := testCard(t)
	c.MouseIn(nil)
	c.startRename()
	c.MouseOut()
	cv.Unfocus()
	if c.pencil.Visible() {
		t.Error("pencil shown on a card the pointer has left")
	}
}

func TestRenameReleasesFocus(t *testing.T) {
	for name, end := range map[string]func(*deviceCard){
		"enter":  func(c *deviceCard) { c.editor.OnSubmitted("x") },
		"escape": func(c *deviceCard) { c.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape}) },
	} {
		c, cv, _ := testCard(t)
		c.startRename()
		end(c)
		if cv.Focused() != nil {
			t.Errorf("%s: hidden editor still has focus", name)
		}
	}
}

func TestFocusMovingToAnotherEditorKeepsIt(t *testing.T) {
	a := test.NewApp()
	a.Settings().SetTheme(darkTheme{})
	var names []string
	act := cardActions{rename: func(_ *deviceCard, n string) { names = append(names, n) }}
	first := newDeviceCard("a", scan.Device{Name: "a"}, deviceStatus{}, act)
	second := newDeviceCard("b", scan.Device{Name: "b"}, deviceStatus{}, act)
	cv := test.NewWindow(container.NewVBox(first, second)).Canvas()

	first.startRename()
	first.editor.SetText("x")
	done := make(chan struct{})
	go func() {
		defer close(done)
		second.startRename() // takes focus from first, as a click would
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("focus change deadlocked")
	}
	if len(names) != 1 || names[0] != "x" {
		t.Errorf("renames = %v", names)
	}
	if cv.Focused() != fyne.Focusable(second.editor) {
		t.Errorf("focus = %T, want the second editor", cv.Focused())
	}
}

func TestSparklineClampsToSparkSamples(t *testing.T) {
	c, _, _ := testCard(t)
	c.spark.set(make([]time.Duration, sparkSamples+5), colOnline)
	if len(c.spark.values) != sparkSamples {
		t.Errorf("kept %d values", len(c.spark.values))
	}
}

func TestLearningMACKeepsOneCard(t *testing.T) {
	u := newTestUI(t)
	byIP := scan.Device{Name: "jetkvm", IP: "192.168.1.21"}
	u.store.SetFavorite(entryOf(byIP), true)
	u.store.SetName(entryOf(byIP), "Office PC")
	u.ensureCard(byIP)

	byIP.MAC = testMAC
	c := u.ensureCard(byIP)

	if len(u.cards) != 1 || u.byKey[testMAC] != c || c.key != testMAC {
		t.Fatalf("cards=%d byKey=%v key=%q", len(u.cards), u.byKey, c.key)
	}
	if c.custom != "Office PC" || !c.favorite {
		t.Errorf("custom=%q favorite=%v", c.custom, c.favorite)
	}
}

func TestLosingMACKeepsOneCard(t *testing.T) {
	u := newTestUI(t)
	d := scan.Device{Name: "jetkvm", IP: "192.168.1.21", MAC: testMAC}
	first := u.ensureCard(d)

	d.MAC = "" // ARP entry aged out before this scan
	if c := u.ensureCard(d); c != first || len(u.cards) != 1 || c.dev.MAC != testMAC {
		t.Fatalf("cards=%d same=%v mac=%q", len(u.cards), c == first, c.dev.MAC)
	}
}

func TestDifferentMACAtSameIPIsAnotherDevice(t *testing.T) {
	u := newTestUI(t)
	u.ensureCard(scan.Device{IP: "192.168.1.21", MAC: testMAC})
	u.ensureCard(scan.Device{IP: "192.168.1.21", MAC: "aa:bb:cc:dd:ee:ff"})
	if len(u.cards) != 2 {
		t.Fatalf("cards=%d", len(u.cards))
	}
}
