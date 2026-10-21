// JetFind scans the local network for JetKVM devices and opens the one you
// double-click in the default browser.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"jetfind/scan"
)

// While macOS waits for the user to grant Local Network access, every
// connection fails; retry for a while instead of reporting "nothing found".
const (
	blockedRetries = 15
	blockedDelay   = 2 * time.Second
	statusInterval = 5 * time.Second
)

//go:embed Icon.png
var iconPNG []byte

var appIcon = fyne.NewStaticResource("Icon.png", iconPNG)

type ui struct {
	app      fyne.App
	subtitle *canvas.Text
	button   *glowButton
	bar      *thinBar
	list     *fyne.Container
	hint     *canvas.Text
	empty    fyne.CanvasObject

	devices []scan.Device
	status  map[string]deviceStatus // keyed by IP
	latest  string                  // newest firmware release on GitHub
	cards   []*deviceCard
	holders map[*deviceCard]fyne.CanvasObject // slide-in wrappers
	cancel  context.CancelFunc
	demo    bool // JETFIND_DEMO=1: show sample devices, touch no network
}

func main() {
	a := app.NewWithID("no.stig.jetfind")
	a.Settings().SetTheme(darkTheme{})
	a.SetIcon(appIcon)
	w := a.NewWindow("JetFind")

	u := &ui{app: a, status: map[string]deviceStatus{}}
	w.SetContent(u.build())
	w.Resize(fyne.NewSize(400, 600))
	w.SetOnClosed(func() {
		if u.cancel != nil {
			u.cancel()
		}
	})
	if os.Getenv("JETFIND_DEMO") != "" {
		u.showDemo()
	} else {
		u.startScan()
		go u.checkRelease()
		go func() {
			for range time.Tick(statusInterval) {
				fyne.Do(u.refreshAll)
			}
		}()
	}
	w.ShowAndRun()
}

// showDemo fills the window with made-up devices, for screenshots that must
// not reveal a real network.
func (u *ui) showDemo() {
	u.demo = true
	u.latest = "0.6.0"
	ms := func(v ...float64) []time.Duration {
		out := make([]time.Duration, len(v))
		for i, x := range v {
			out[i] = time.Duration(x * float64(time.Millisecond))
		}
		return out
	}
	day := 24 * time.Hour
	devices := []struct {
		dev  scan.Device
		info scan.Info
		mac  string
		hist []time.Duration
	}{
		{scan.Device{Name: "jetkvm-office", IP: "192.168.1.21"},
			scan.Info{Version: "0.6.0", Uptime: 12*day + 5*time.Hour, IsSetup: true, ClockKnown: true, ClockSynced: true},
			"30:52:53:00:1a:2b", ms(3, 3, 4, 3, 2, 3, 5, 3, 3, 4, 3, 3, 2, 3, 3, 4, 3, 3)},
		{scan.Device{Name: "jetkvm-rack", IP: "192.168.1.22"},
			scan.Info{Version: "0.6.0", Uptime: 3*day + 9*time.Hour, IsSetup: true, ClockKnown: true, ClockSynced: true, ActiveClient: "192.168.1.50"},
			"30:52:53:00:3c:4d", ms(4, 5, 4, 6, 9, 5, 4, 4, 5, 7, 4, 4, 5, 4, 6, 5, 4, 5)},
		{scan.Device{Name: "jetkvm-lab", IP: "192.168.1.23"},
			scan.Info{Version: "0.5.9", Uptime: 41*day + 2*time.Hour, IsSetup: true, CloudConnected: true, ClockKnown: true, ClockSynced: true},
			"30:52:53:00:5e:6f", ms(6, 5, 7, 6, 12, 8, 6, 6, 7, 6, 5, 6, 9, 7, 6, 6, 5, 6)},
	}
	for _, d := range devices {
		st := d.hist[len(d.hist)-1]
		d.info.Latency = st
		u.status[d.dev.IP] = deviceStatus{info: d.info, known: true, online: true, history: d.hist, mac: d.mac, latest: u.latest}
		u.add(d.dev)
	}
	u.finish("192.168.1.0/24")
}

// checkRelease looks up the newest firmware on GitHub, retrying hourly so
// a long-running window still notices new releases.
func (u *ui) checkRelease() {
	for {
		if v, err := scan.LatestRelease(context.Background()); err == nil {
			fyne.Do(func() {
				u.latest = v
				u.refreshAll()
			})
		}
		time.Sleep(time.Hour)
	}
}

// refreshAll fetches live status for every listed device.
func (u *ui) refreshAll() {
	for _, d := range u.devices {
		u.refresh(d)
	}
}

// refresh fetches live status for d in the background and updates its card.
func (u *ui) refresh(d scan.Device) {
	if u.demo {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		info, online := scan.FetchInfo(ctx, d)
		mac := scan.MACAddress(d.IP)
		fyne.Do(func() {
			hist := u.status[d.IP].history
			if online {
				hist = append(hist, info.Latency)
				if len(hist) > sparkSamples {
					hist = hist[len(hist)-sparkSamples:]
				}
			}
			if mac == "" {
				mac = u.status[d.IP].mac
			}
			st := deviceStatus{info: info, known: true, online: online, history: hist, mac: mac, latest: u.latest}
			u.status[d.IP] = st
			for _, c := range u.cards {
				if c.dev.IP == d.IP {
					c.setStatus(st)
				}
			}
		})
	}()
}

func (u *ui) build() fyne.CanvasObject {
	title := canvas.NewText("JetFind", colText)
	title.TextSize = 22
	title.TextStyle = fyne.TextStyle{Bold: true}
	u.subtitle = canvas.NewText("", colMuted)
	u.subtitle.TextSize = 11
	logo := canvas.NewImageFromResource(appIcon)
	logo.FillMode = canvas.ImageFillContain

	u.button = newGlowButton("Scan", theme.ViewRefreshIcon(), u.startScan)

	header := container.NewBorder(nil, nil,
		container.NewHBox(
			container.NewCenter(container.NewGridWrap(fyne.NewSize(44, 44), logo)),
			container.New(layout.NewCustomPaddedVBoxLayout(2), title, u.subtitle)),
		container.NewCenter(u.button))

	u.bar = newThinBar()
	u.list = container.New(layout.NewCustomPaddedVBoxLayout(10))

	emptyTitle := canvas.NewText("No JetKVMs found", colText)
	emptyTitle.TextStyle = fyne.TextStyle{Bold: true}
	emptyTitle.Alignment = fyne.TextAlignCenter
	help := "Make sure the devices are powered on and that you're on the same network."
	if runtime.GOOS == "darwin" {
		help += "\n\nOn a Mac, JetFind needs Local Network access (System Settings › Privacy & Security › Local Network)."
	}
	emptyBody := widget.NewLabel(help)
	emptyBody.Wrapping = fyne.TextWrapWord
	emptyBody.Alignment = fyne.TextAlignCenter
	emptyBody.Importance = widget.LowImportance
	u.empty = container.NewVBox(layout.NewSpacer(), emptyTitle, emptyBody, layout.NewSpacer())
	u.empty.Hide()

	u.hint = canvas.NewText("Double-click to open in your browser", colFaint)
	u.hint.TextSize = 11
	u.hint.Alignment = fyne.TextAlignCenter
	u.hint.Hide()

	body := container.NewStack(container.NewVScroll(container.New(layout.NewCustomPaddedLayout(4, 20, 2, 2), u.list)), u.empty)
	top := container.New(layout.NewCustomPaddedLayout(0, 6, 0, 0), container.NewVBox(header, u.bar))
	content := container.NewBorder(top, u.hint, nil, nil, body)

	return container.New(layout.NewCustomPaddedLayout(14, 12, 16, 16), content)
}

// startScan clears the list and scans every local subnet in the background.
func (u *ui) startScan() {
	if u.cancel != nil {
		u.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel

	for _, c := range u.cards {
		c.stop()
	}
	u.devices, u.cards, u.holders = nil, nil, nil
	u.status = map[string]deviceStatus{}
	u.list.RemoveAll()
	u.empty.Hide()
	u.hint.Hide()
	u.button.setBusy(true, "Scanning…")
	u.bar.set(0)
	u.bar.Show()
	u.bar.shimmer(true)

	nets, err := scan.LocalSubnets()
	if err != nil || len(nets) == 0 {
		u.finish("")
		u.setSubtitle("No network found")
		return
	}
	names := make([]string, len(nets))
	for i, n := range nets {
		names[i] = n.String()
	}
	netLabel := strings.Join(names, ", ")
	u.setSubtitle("Scanning…")

	go func() {
		for attempt := 0; ; attempt++ {
			blocked := scan.Run(ctx, nets,
				func(done, total int) {
					fyne.Do(func() {
						if ctx.Err() != nil {
							return
						}
						u.bar.set(float32(done) / float32(total))
						u.setSubtitle(fmt.Sprintf("Scanning… %d of %d addresses", done, total))
					})
				},
				func(d scan.Device) {
					fyne.Do(func() {
						if ctx.Err() == nil {
							u.add(d)
						}
					})
				})
			if !blocked || attempt == blockedRetries {
				break
			}
			fyne.Do(func() { u.setSubtitle("Waiting for local network access…") })
			select {
			case <-ctx.Done():
				return
			case <-time.After(blockedDelay):
			}
		}
		fyne.Do(func() {
			if ctx.Err() == nil {
				u.finish(netLabel)
			}
		})
	}()
}

func (u *ui) finish(netLabel string) {
	u.bar.shimmer(false)
	u.bar.Hide()
	u.button.setBusy(false, "Scan")
	switch n := len(u.devices); n {
	case 0:
		u.setSubtitle("No devices found")
		u.empty.Show()
	case 1:
		u.setSubtitle("1 device found · " + netLabel)
	default:
		u.setSubtitle(fmt.Sprintf("%d devices found · %s", n, netLabel))
	}
}

func (u *ui) setSubtitle(s string) {
	u.subtitle.Text = s
	u.subtitle.Refresh()
}

// add inserts a device card, keeping the list sorted by IP address.
func (u *ui) add(d scan.Device) {
	u.devices = append(u.devices, d)
	sort.Slice(u.devices, func(i, j int) bool { return ipLess(u.devices[i].IP, u.devices[j].IP) })
	u.cards = append(u.cards, newDeviceCard(d, u.status[d.IP], u.open))
	sort.Slice(u.cards, func(i, j int) bool { return ipLess(u.cards[i].dev.IP, u.cards[j].dev.IP) })
	if u.holders == nil {
		u.holders = map[*deviceCard]fyne.CanvasObject{}
	}
	for _, c := range u.cards {
		if u.holders[c] == nil {
			u.holders[c] = slideIn(c)
		}
	}
	objs := make([]fyne.CanvasObject, len(u.cards))
	for i, c := range u.cards {
		objs[i] = u.holders[c]
	}
	u.list.Objects = objs
	u.list.Refresh()
	u.hint.Show()
	u.refresh(d)
}

func (u *ui) open(d scan.Device) {
	if link, err := url.Parse(d.URL); err == nil {
		u.app.OpenURL(link)
	}
}

func ipLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		var x, y int
		fmt.Sscan(pa[i], &x)
		fmt.Sscan(pb[i], &y)
		if x != y {
			return x < y
		}
	}
	return a < b
}

// thinBar is a 3px gradient progress line with a moving shimmer.
type thinBar struct {
	widget.BaseWidget
	track *canvas.Rectangle
	fill  *canvas.LinearGradient
	shine *canvas.LinearGradient
	value float32
	phase float32
	anim  *fyne.Animation
}

func newThinBar() *thinBar {
	b := &thinBar{
		track: canvas.NewRectangle(colBorder),
		fill:  canvas.NewHorizontalGradient(colAccent, colAccent2),
		shine: canvas.NewHorizontalGradient(color.Transparent, withAlpha(color.White, 0)),
	}
	b.track.CornerRadius = 1.5
	b.ExtendBaseWidget(b)
	return b
}

func (b *thinBar) set(v float32) {
	b.value = v
	b.Refresh()
}

// shimmer starts or stops a highlight sweeping along the filled part.
func (b *thinBar) shimmer(on bool) {
	if b.anim != nil {
		b.anim.Stop()
		b.anim = nil
	}
	if !on {
		return
	}
	b.anim = fyne.NewAnimation(1200*time.Millisecond, func(v float32) {
		b.phase = v
		b.Refresh()
	})
	b.anim.RepeatCount = fyne.AnimationRepeatForever
	b.anim.Curve = fyne.AnimationEaseInOut
	b.anim.Start()
}

func (b *thinBar) CreateRenderer() fyne.WidgetRenderer { return &thinBarRenderer{b} }

type thinBarRenderer struct{ b *thinBar }

func (r *thinBarRenderer) Layout(s fyne.Size) {
	b := r.b
	b.track.Move(fyne.NewPos(0, 6))
	b.track.Resize(fyne.NewSize(s.Width, 3))
	w := s.Width * b.value
	b.fill.Move(fyne.NewPos(0, 6))
	b.fill.Resize(fyne.NewSize(w, 3))

	const shineW = 60
	b.shine.StartColor = withAlpha(color.White, 0)
	b.shine.EndColor = withAlpha(color.White, 0.7)
	x := b.phase*(w+shineW) - shineW
	sw := float32(shineW)
	if x < 0 {
		sw += x
		x = 0
	}
	sw = min(sw, w-x)
	if sw <= 0 {
		b.shine.Hide()
		return
	}
	b.shine.Show()
	b.shine.Move(fyne.NewPos(x, 6))
	b.shine.Resize(fyne.NewSize(sw, 3))
}
func (r *thinBarRenderer) MinSize() fyne.Size { return fyne.NewSize(10, 15) }
func (r *thinBarRenderer) Refresh()           { r.Layout(r.b.Size()); canvas.Refresh(r.b) }
func (r *thinBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.track, r.b.fill, r.b.shine}
}
func (r *thinBarRenderer) Destroy() {}
