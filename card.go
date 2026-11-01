package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"jetfind/scan"
	"jetfind/store"
)

// deviceStatus is the latest live info for a device; known is false until
// the first fetch completes, online is false when the device stopped answering.
type deviceStatus struct {
	info          scan.Info
	known, online bool
	history       []time.Duration // recent latencies, oldest first
	mac           string          // from the local ARP table
	latest        string          // newest firmware on GitHub, if known
	lastSeen      time.Time       // when the device last answered
}

// iconTile is the shared gradient background for device icons (2x for HiDPI).
var iconTile = gradientTile(76, 18, colAccent, colAccent2)

// cardActions are the callbacks a card raises.
type cardActions struct {
	open     func(*deviceCard)
	favorite func(*deviceCard)
	rename   func(c *deviceCard, name string)
}

// deviceCard shows one JetKVM on a shaded panel. A double tap opens it.
type deviceCard struct {
	widget.BaseWidget
	key      string // store.Key of the device
	dev      scan.Device
	status   deviceStatus
	favorite bool
	custom   string // user-given name, empty for the hostname
	hovered  bool
	act      cardActions

	nameTx *canvas.Text
	ipTx   *canvas.Text
	editor *nameEntry
	pencil *iconButton
	star   *iconButton

	glass   *canvas.Shader
	dot     *statusDot
	spark   *sparkline
	ping    *canvas.Text
	details *canvas.Text
	mac     *canvas.Text
	clock   *canvas.Text
	pills   *fyne.Container
	inUse   *fyne.Container
	inUseTx *canvas.Text
	update  *fyne.Container
	updTx   *canvas.Text

	hoverAnim, flashAnim, rippleAnim *fyne.Animation
}

const cardRadius = 14

func newDeviceCard(key string, dev scan.Device, st deviceStatus, act cardActions) *deviceCard {
	c := &deviceCard{key: key, dev: dev, status: st, act: act}
	c.ExtendBaseWidget(c)
	return c
}

// title is the name shown in bold: the custom name, else the hostname.
func (c *deviceCard) title() string {
	if c.custom != "" {
		return c.custom
	}
	return c.dev.Name
}

// setIdentity updates what the card shows about who the device is.
func (c *deviceCard) setIdentity(dev scan.Device, custom string, favorite bool) {
	c.dev, c.custom, c.favorite = dev, custom, favorite
	if c.nameTx == nil {
		return
	}
	c.nameTx.Text = c.title()
	c.ipTx.Text = dev.IP
	if custom != "" {
		c.ipTx.Text = dev.IP + " · " + dev.Name
	}
	c.star.setIcon(starOff)
	if favorite {
		c.star.setIcon(starOn)
	}
	c.nameTx.Refresh()
	c.ipTx.Refresh()
}

// startRename swaps the name for an editor.
func (c *deviceCard) startRename() {
	c.editor.start(c.custom)
	c.editor.SetPlaceHolder(c.dev.Name)
	c.nameTx.Hide()
	c.pencil.Hide()
	c.editor.Show()
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		cv.Focus(c.editor)
	}
}

func (c *deviceCard) endRename(name string, ok bool) {
	c.editor.Hide()
	c.nameTx.Show()
	if c.hovered {
		c.pencil.Show() // MouseIn won't fire again for a pointer already inside
	}
	// Fyne keeps focus on hidden widgets.
	if cv := fyne.CurrentApp().Driver().CanvasForObject(c); cv != nil {
		cv.Unfocus()
	}
	if ok {
		c.act.rename(c, name)
	}
}

func (c *deviceCard) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.Transparent)
	shadow.CornerRadius = cardRadius
	shadow.Shadow = canvas.Shadow{
		Color:      color.NRGBA{0, 0, 0, 0x55},
		BlurRadius: 22,
		Offset:     fyne.NewPos(0, 8),
		Spread:     -4,
	}
	c.glass = newGlass(cardRadius)

	tile := canvas.NewImageFromImage(iconTile)
	icon := canvas.NewImageFromResource(theme.NewColoredResource(theme.ComputerIcon(), theme.ColorNameForeground))
	icon.FillMode = canvas.ImageFillContain
	iconBox := container.NewGridWrap(fyne.NewSize(38, 38),
		container.NewStack(tile, container.New(layout.NewCustomPaddedLayout(8, 8, 8, 8), icon)))

	c.nameTx = canvas.NewText("", colText)
	c.nameTx.TextStyle = fyne.TextStyle{Bold: true}
	c.nameTx.TextSize = 14
	c.editor = newNameEntry()
	c.editor.onDone = c.endRename
	c.editor.Hide()
	c.pencil = newIconButton(pencil, 14, c.startRename)
	c.pencil.Hide()
	name := container.NewBorder(nil, nil, nil,
		container.NewCenter(c.pencil),
		container.NewStack(container.NewHBox(c.nameTx), c.editor))
	c.ipTx = canvas.NewText("", colMuted)
	c.ipTx.TextStyle = fyne.TextStyle{Monospace: true}
	c.ipTx.TextSize = 11
	c.star = newIconButton(starOff, 18, func() { c.act.favorite(c) })
	c.details = canvas.NewText("", colMuted)
	c.details.TextSize = 11

	c.mac = canvas.NewText("", colMuted)
	c.mac.TextSize = 10
	c.mac.TextStyle = fyne.TextStyle{Monospace: true}
	c.clock = canvas.NewText("", colMuted)
	c.clock.TextSize = 10
	hw := container.New(layout.NewCustomPaddedHBoxLayout(8), c.mac, c.clock)

	c.inUse, c.inUseTx = newPill(colAccent, colAccentText)
	c.update, c.updTx = newPill(colWarn, colWarn)
	c.pills = container.New(layout.NewCustomPaddedHBoxLayout(6), c.inUse, c.update)

	texts := container.New(layout.NewCustomPaddedVBoxLayout(2), name, c.ipTx, c.details, hw,
		container.New(layout.NewCustomPaddedLayout(3, 0, 0, 0), c.pills))

	c.dot = newStatusDot()
	c.ping = canvas.NewText("", colMuted)
	c.ping.TextSize = 11
	c.ping.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	c.spark = newSparkline()
	right := container.New(layout.NewCustomPaddedVBoxLayout(2),
		container.NewHBox(layout.NewSpacer(), c.dot, c.ping, container.NewCenter(c.star)),
		c.spark)

	row := container.NewBorder(nil, nil,
		container.NewHBox(container.NewVBox(iconBox), layout.NewSpacer()),
		container.NewVBox(right),
		texts)
	padded := container.New(layout.NewCustomPaddedLayout(13, 13, 14, 12), row)

	c.setIdentity(c.dev, c.custom, c.favorite)
	c.applyStatus()
	return widget.NewSimpleRenderer(container.New(shaderStack{}, shadow, c.glass, padded))
}

// stop halts running animations; call before discarding the card.
func (c *deviceCard) stop() {
	for _, a := range []*fyne.Animation{c.hoverAnim, c.flashAnim, c.rippleAnim} {
		if a != nil {
			a.Stop()
		}
	}
	if c.dot != nil {
		c.dot.stop()
	}
}

// setStatus updates the live info shown on the card.
// A fresh successful ping makes the status dot pulse once.
func (c *deviceCard) setStatus(st deviceStatus) {
	fresh := st.online && st.lastSeen.After(c.status.lastSeen)
	c.status = st
	c.applyStatus()
	if fresh && c.dot != nil {
		c.dot.pulse()
	}
}

func (c *deviceCard) applyStatus() {
	if c.dot == nil {
		return
	}
	st := c.status
	var col color.Color
	switch {
	case !st.known:
		col, c.ping.Text = colFaint, "…"
		c.dot.set(col)
	case !st.online:
		col, c.ping.Text = colOffline, "offline"
		c.dot.set(col)
	default:
		col, c.ping.Text = latencyColor(st.info.Latency), formatLatency(st.info.Latency)
		c.dot.set(col)
	}
	c.ping.Color = col
	c.spark.set(st.history, col)

	var parts []string
	c.details.Color = colMuted
	c.nameTx.Color = colText
	if st.known && !st.online {
		c.nameTx.Color = colMuted
		parts = append(parts, "last seen "+store.Ago(st.lastSeen, time.Now()))
	}
	if st.known && st.online {
		if st.info.SetupKnown && !st.info.IsSetup {
			parts = append(parts, "Not set up")
			c.details.Color = colWarn
		}
		if v := st.info.Version; v != "" {
			if st.latest != "" && !scan.NewerVersion(st.latest, v) {
				v += " (latest)"
			}
			parts = append(parts, "v"+v)
		}
		if st.info.Uptime > 0 {
			parts = append(parts, "up "+formatUptime(st.info.Uptime))
		}
		if st.info.CloudConnected {
			parts = append(parts, "cloud")
		}
	}
	c.details.Text = strings.Join(parts, " · ")

	c.mac.Text = st.mac
	c.clock.Text = ""
	if st.known && st.online {
		if st.info.ClockKnown {
			c.clock.Text, c.clock.Color = "NTP ok", colOnline
			if !st.info.ClockSynced {
				c.clock.Text, c.clock.Color = "Clock not synced", colWarn
			}
		}
	}

	showPill(c.inUse, c.inUseTx, st.online && st.info.ActiveClient != "", "In use by "+st.info.ActiveClient)
	showPill(c.update, c.updTx, st.online && scan.NewerVersion(st.latest, st.info.Version), "Update "+st.latest+" available")
	if c.inUse.Visible() || c.update.Visible() {
		c.pills.Show()
	} else {
		c.pills.Hide()
	}

	for _, t := range []*canvas.Text{c.nameTx, c.ping, c.details, c.mac, c.clock} {
		t.Refresh()
	}
}

// newPill makes a small rounded badge tinted with col.
func newPill(col, text color.Color) (*fyne.Container, *canvas.Text) {
	bg := canvas.NewRectangle(withAlpha(col, 0.20))
	bg.CornerRadius = 8
	bg.StrokeColor = withAlpha(text, 0.35)
	bg.StrokeWidth = 1
	tx := canvas.NewText("", text)
	tx.TextSize = 10
	tx.TextStyle = fyne.TextStyle{Bold: true}
	pill := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 8, 8), tx))
	pill.Hide()
	return pill, tx
}

func showPill(pill *fyne.Container, tx *canvas.Text, show bool, text string) {
	if !show {
		pill.Hide()
		return
	}
	tx.Text = text
	tx.Refresh()
	pill.Show()
}

func latencyColor(d time.Duration) color.Color {
	switch {
	case d < 30*time.Millisecond:
		return colOnline
	case d < 150*time.Millisecond:
		return colWarn
	}
	return colOffline
}

func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return "<1 ms"
	}
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// DoubleTapped flashes the card and opens the device.
func (c *deviceCard) DoubleTapped(*fyne.PointEvent) {
	c.glass.Uniforms["flash"] = 1
	animateUniform(&c.flashAnim, c.glass, "flash", 0, 450*time.Millisecond)
	c.act.open(c)
}

func (c *deviceCard) MouseIn(*desktop.MouseEvent) {
	c.hovered = true
	animateUniform(&c.hoverAnim, c.glass, "hover", 1, 200*time.Millisecond)
	if !c.editor.Visible() {
		c.pencil.Show()
	}
}

func (c *deviceCard) MouseMoved(*desktop.MouseEvent) {}

// MouseDown sends a wave out from the press point. It reacts on press
// rather than Tapped, which Fyne delays while it waits for a double tap.
func (c *deviceCard) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button != desktop.MouseButtonPrimary {
		return
	}
	c.glass.Uniforms["rx"], c.glass.Uniforms["ry"] = ev.Position.X, ev.Position.Y
	c.glass.Uniforms["ripple"] = 0
	animateUniform(&c.rippleAnim, c.glass, "ripple", 1, 750*time.Millisecond)
}

func (c *deviceCard) MouseUp(*desktop.MouseEvent) {}

func (c *deviceCard) MouseOut() {
	c.hovered = false
	animateUniform(&c.hoverAnim, c.glass, "hover", 0, 300*time.Millisecond)
	c.pencil.Hide()
}

func (c *deviceCard) Cursor() desktop.Cursor { return desktop.PointerCursor }
