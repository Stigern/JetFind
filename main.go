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
	"jetfind/store"
)

// While macOS waits for the user to grant Local Network access, every
// connection fails; retry for a while instead of reporting "nothing found".
const (
	blockedRetries = 15
	blockedDelay   = 2 * time.Second
	statusInterval = 5 * time.Second
)

// The 1024 px Icon.png is only for the app bundle; a 256 px copy is plenty
// for the window and header and is much cheaper to decode at startup.
//
//go:embed docs/icon.png
var iconPNG []byte

var appIcon = fyne.NewStaticResource("icon.png", iconPNG)

type ui struct {
	app      fyne.App
	subtitle *canvas.Text
	button   *glowButton
	bar      *thinBar
	list     *fyne.Container
	hint     *canvas.Text
	empty    fyne.CanvasObject

	store   *store.Store
	cards   []*deviceCard                     // favourites first, then by IP
	byKey   map[string]*deviceCard            // store.Key → card
	holders map[*deviceCard]fyne.CanvasObject // slide-in wrappers
	latest  string                            // newest firmware release on GitHub
	found   int                               // devices found by the current scan
	cancel  context.CancelFunc
	demo    bool // JETFIND_DEMO=1: show sample devices, touch no network
}

func main() {
	a := app.NewWithID("no.stig.jetfind")
	a.Settings().SetTheme(darkTheme{})
	a.SetIcon(appIcon)
	w := a.NewWindow("JetFind")

	u := &ui{app: a, byKey: map[string]*deviceCard{}, holders: map[*deviceCard]fyne.CanvasObject{}}
	w.SetContent(u.build())
	w.Resize(fyne.NewSize(400, 600))
	w.SetOnClosed(func() {
		if u.cancel != nil {
			u.cancel()
		}
	})
	if os.Getenv("JETFIND_DEMO") != "" {
		u.store = store.Open(memPrefs{})
		u.showDemo()
	} else {
		u.store = store.Open(a.Preferences())
		u.loadFavorites()
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

// memPrefs keeps demo-mode changes out of the real preferences.
type memPrefs map[string]string

func (m memPrefs) String(k string) string { return m[k] }
func (m memPrefs) SetString(k, v string)  { m[k] = v }

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
		dev    scan.Device
		info   scan.Info
		online bool
		fav    bool
		name   string
		hist   []time.Duration
	}{
		{scan.Device{Name: "jetkvm-office", IP: "192.168.1.21", MAC: "30:52:53:00:1a:2b"},
			scan.Info{Version: "0.6.0", Uptime: 12*day + 5*time.Hour, IsSetup: true, ClockKnown: true, ClockSynced: true},
			true, true, "Office PC", ms(3, 3, 4, 3, 2, 3, 5, 3, 3, 4, 3, 3, 2, 3, 3, 4, 3, 3)},
		{scan.Device{Name: "jetkvm-rack", IP: "192.168.1.22", MAC: "30:52:53:00:3c:4d"},
			scan.Info{Version: "0.6.0", Uptime: 3*day + 9*time.Hour, IsSetup: true, ClockKnown: true, ClockSynced: true, ActiveClient: "192.168.1.50"},
			true, true, "", ms(4, 5, 4, 6, 9, 5, 4, 4, 5, 7, 4, 4, 5, 4, 6, 5, 4, 5)},
		{scan.Device{Name: "jetkvm-garage", IP: "192.168.1.24", MAC: "30:52:53:00:7a:8b"},
			scan.Info{}, false, true, "Garage NAS", nil},
		{scan.Device{Name: "jetkvm-lab", IP: "192.168.1.23", MAC: "30:52:53:00:5e:6f"},
			scan.Info{Version: "0.5.9", Uptime: 41*day + 2*time.Hour, IsSetup: true, CloudConnected: true, ClockKnown: true, ClockSynced: true},
			true, false, "", ms(6, 5, 7, 6, 12, 8, 6, 6, 7, 6, 5, 6, 9, 7, 6, 6, 5, 6)},
	}
	for _, d := range devices {
		e := entryOf(d.dev)
		u.store.SetFavorite(e, d.fav)
		u.store.SetName(e, d.name)
		c := u.ensureCard(d.dev)
		st := deviceStatus{known: true, online: d.online, mac: d.dev.MAC, latest: u.latest, lastSeen: time.Now().Add(-3 * time.Hour)}
		if d.online {
			d.info.Latency = d.hist[len(d.hist)-1]
			st.info, st.history, st.lastSeen = d.info, d.hist, time.Now()
			u.found++
		}
		c.setStatus(st)
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

// loadFavorites shows saved favourites straight away, before any scan.
func (u *ui) loadFavorites() {
	for _, e := range u.store.Favorites() {
		c := u.ensureCard(scan.Device{Name: e.Hostname, IP: e.IP, URL: "http://" + e.IP, MAC: e.MAC})
		c.status.lastSeen = e.LastSeen
		u.refresh(c)
	}
}

// refreshAll fetches live status for every listed device.
func (u *ui) refreshAll() {
	for _, c := range u.cards {
		u.refresh(c)
	}
}

// refresh fetches live status for c in the background and updates it.
func (u *ui) refresh(c *deviceCard) {
	if u.demo {
		return
	}
	d := c.dev
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		info, online := scan.FetchInfo(ctx, d)
		mac := scan.MACAddress(d.IP)
		// A favourite may have moved; then its old IP belongs to someone else.
		if online && d.MAC != "" && mac != "" && !strings.EqualFold(mac, d.MAC) {
			online = false
		}
		fyne.Do(func() {
			if u.byKey[c.key] != c || c.dev.IP != d.IP {
				return // card removed or moved while we were fetching
			}
			st := c.status
			st.info, st.known, st.online, st.latest = info, true, online, u.latest
			if mac != "" && online {
				st.mac = mac
			}
			if online {
				st.lastSeen = time.Now()
				st.history = append(st.history, info.Latency)
				if len(st.history) > sparkSamples {
					st.history = st.history[len(st.history)-sparkSamples:]
				}
				u.store.Seen(entryOf(d), st.lastSeen)
			}
			c.setStatus(st)
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

	u.hint = canvas.NewText("Double-click to open · star to keep as favorite", colFaint)
	u.hint.TextSize = 11
	u.hint.Alignment = fyne.TextAlignCenter
	u.hint.Hide()

	body := container.NewStack(container.NewVScroll(container.New(layout.NewCustomPaddedLayout(4, 20, 2, 2), u.list)), u.empty)
	top := container.New(layout.NewCustomPaddedLayout(0, 6, 0, 0), container.NewVBox(header, u.bar))
	content := container.NewBorder(top, u.hint, nil, nil, body)

	return container.New(layout.NewCustomPaddedLayout(14, 12, 16, 16), content)
}

// startScan drops non-favourite cards and scans every local subnet in the
// background. Favourites stay and are updated if found.
func (u *ui) startScan() {
	if u.cancel != nil {
		u.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.cancel = cancel

	for _, c := range append([]*deviceCard(nil), u.cards...) {
		if !c.favorite {
			u.remove(c)
		}
	}
	u.found = 0
	u.relayout()
	u.empty.Hide()
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
	if len(u.cards) == 0 {
		u.empty.Show()
	}
	switch u.found {
	case 0:
		u.setSubtitle("No devices found")
	case 1:
		u.setSubtitle("1 device found · " + netLabel)
	default:
		u.setSubtitle(fmt.Sprintf("%d devices found · %s", u.found, netLabel))
	}
}

func (u *ui) setSubtitle(s string) {
	u.subtitle.Text = s
	u.subtitle.Refresh()
}

// add shows a device found by the scan, updating its card if it already
// has one (a favourite, possibly at a new IP).
func (u *ui) add(d scan.Device) {
	u.found++
	c := u.ensureCard(d)
	u.store.Seen(entryOf(c.dev), time.Now())
	u.refresh(c)
}

// ensureCard returns the card for d, creating it if needed, and updates the
// card's address and saved name/favourite state.
func (u *ui) ensureCard(d scan.Device) *deviceCard {
	if d.URL == "" {
		d.URL = "http://" + d.IP
	}
	// ARP can lag behind a scan, so the same device may arrive with or
	// without its MAC. A MAC that differs from the card's is another device.
	if c := u.cardAt(d.IP); c != nil {
		if d.MAC == "" {
			d.MAC = c.dev.MAC
		} else if c.dev.MAC == "" {
			delete(u.byKey, c.key)
			c.key = store.Key(d.MAC, d.IP)
			u.byKey[c.key] = c
		}
	}
	u.store.Adopt(entryOf(d))
	key := store.Key(d.MAC, d.IP)
	saved, _ := u.store.Get(key)
	if d.Name == "" || d.Name == "JetKVM" && saved.Hostname != "" {
		d.Name = saved.Hostname
	}
	if d.Name == "" {
		d.Name = "JetKVM"
	}
	c := u.byKey[key]
	if c == nil {
		c = newDeviceCard(key, d, deviceStatus{mac: d.MAC, lastSeen: saved.LastSeen}, cardActions{
			open:     u.open,
			favorite: u.toggleFavorite,
			rename:   u.rename,
		})
		u.byKey[key] = c
		u.cards = append(u.cards, c)
		u.holders[c] = slideIn(c)
	}
	c.setIdentity(d, saved.Name, saved.Favorite)
	u.relayout()
	return c
}

// cardAt returns the card currently at ip, if any.
func (u *ui) cardAt(ip string) *deviceCard {
	for _, c := range u.cards {
		if c.dev.IP == ip {
			return c
		}
	}
	return nil
}

func (u *ui) remove(c *deviceCard) {
	c.stop()
	delete(u.byKey, c.key)
	delete(u.holders, c)
	for i, x := range u.cards {
		if x == c {
			u.cards = append(u.cards[:i], u.cards[i+1:]...)
			break
		}
	}
}

// relayout sorts the cards (favourites first, then by IP) and redraws.
func (u *ui) relayout() {
	sort.SliceStable(u.cards, func(i, j int) bool {
		a, b := u.cards[i], u.cards[j]
		if a.favorite != b.favorite {
			return a.favorite
		}
		return ipLess(a.dev.IP, b.dev.IP)
	})
	objs := make([]fyne.CanvasObject, len(u.cards))
	for i, c := range u.cards {
		objs[i] = u.holders[c]
	}
	u.list.Objects = objs
	u.list.Refresh()
	if len(u.cards) > 0 {
		u.hint.Show()
		u.empty.Hide()
	} else {
		u.hint.Hide()
	}
}

func (u *ui) toggleFavorite(c *deviceCard) {
	u.store.SetFavorite(entryOf(c.dev), !c.favorite)
	c.setIdentity(c.dev, c.custom, !c.favorite)
	u.relayout()
}

func (u *ui) rename(c *deviceCard, name string) {
	u.store.SetName(entryOf(c.dev), name)
	saved, _ := u.store.Get(c.key)
	c.setIdentity(c.dev, saved.Name, c.favorite)
}

func (u *ui) open(c *deviceCard) {
	if link, err := url.Parse(c.dev.URL); err == nil {
		u.app.OpenURL(link)
	}
}

// entryOf describes d for the store.
func entryOf(d scan.Device) store.Entry {
	host := d.Name
	if host == "JetKVM" {
		host = ""
	}
	return store.Entry{MAC: d.MAC, IP: d.IP, Hostname: host}
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
