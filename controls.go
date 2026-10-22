package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

var (
	starOn = fyne.NewStaticResource("star-on.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="#f5b73b" d="M12 2.6l2.9 6 6.6.8-4.9 4.6 1.3 6.5L12 17.3 6.1 20.5l1.3-6.5L2.5 9.4l6.6-.8z"/></svg>`))
	starOff = fyne.NewStaticResource("star-off.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="none" stroke="#5d6372" stroke-width="1.6" stroke-linejoin="round" d="M12 2.6l2.9 6 6.6.8-4.9 4.6 1.3 6.5L12 17.3 6.1 20.5l1.3-6.5L2.5 9.4l6.6-.8z"/></svg>`))
	pencil = fyne.NewStaticResource("pencil.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
<path fill="none" stroke="#a3a9ba" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16z M13.5 6.5l4 4"/></svg>`))
)

// iconButton is a small clickable icon. It is deliberately not Hoverable,
// so pointing at it does not end the hover state of the card around it.
type iconButton struct {
	widget.BaseWidget
	img   *canvas.Image
	size  float32
	onTap func()
}

func newIconButton(res fyne.Resource, size float32, onTap func()) *iconButton {
	b := &iconButton{img: canvas.NewImageFromResource(res), size: size, onTap: onTap}
	b.img.FillMode = canvas.ImageFillContain
	b.ExtendBaseWidget(b)
	return b
}

func (b *iconButton) setIcon(res fyne.Resource) {
	b.img.Resource = res
	b.img.Refresh()
}

func (b *iconButton) CreateRenderer() fyne.WidgetRenderer { return widget.NewSimpleRenderer(b.img) }
func (b *iconButton) MinSize() fyne.Size                  { return fyne.NewSquareSize(b.size) }
func (b *iconButton) Tapped(*fyne.PointEvent)             { b.onTap() }
func (b *iconButton) Cursor() desktop.Cursor              { return desktop.PointerCursor }

// nameEntry is a one-line editor that commits on Enter or focus loss and
// cancels on Escape.
type nameEntry struct {
	widget.Entry
	onDone func(name string, ok bool)
	done   bool
}

func newNameEntry() *nameEntry {
	e := &nameEntry{}
	e.ExtendBaseWidget(e)
	e.OnSubmitted = func(s string) { e.finish(s, true) }
	return e
}

func (e *nameEntry) start(text string) {
	e.done = false
	e.SetText(text)
}

func (e *nameEntry) finish(s string, ok bool) {
	if e.done {
		return
	}
	e.done = true
	e.onDone(s, ok)
}

func (e *nameEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		e.finish("", false)
		return
	}
	e.Entry.TypedKey(k)
}

func (e *nameEntry) FocusLost() {
	e.Entry.FocusLost()
	e.finish(e.Text, true)
}
