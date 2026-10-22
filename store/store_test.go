package store

import (
	"testing"
	"time"
)

// memPrefs is an in-memory stand-in for fyne.Preferences.
type memPrefs map[string]string

func (m memPrefs) String(k string) string { return m[k] }
func (m memPrefs) SetString(k, v string)  { m[k] = v }

var dev = Entry{MAC: "30:52:53:00:1A:2B", IP: "192.168.1.21", Hostname: "jetkvm-office"}

func TestKey(t *testing.T) {
	if k := Key("30:52:53:00:1A:2B", "192.168.1.21"); k != "30:52:53:00:1a:2b" {
		t.Errorf("Key with MAC = %q", k)
	}
	if k := Key("", "192.168.1.21"); k != "ip:192.168.1.21" {
		t.Errorf("Key without MAC = %q", k)
	}
}

func TestFavoriteAndNamePersist(t *testing.T) {
	p := memPrefs{}
	s := Open(p)
	s.SetFavorite(dev, true)
	s.SetName(dev, "  Office PC  ")

	s2 := Open(p)
	e, ok := s2.Get(Key(dev.MAC, dev.IP))
	if !ok || !e.Favorite || e.Name != "Office PC" || e.IP != dev.IP || e.Hostname != dev.Hostname {
		t.Fatalf("reloaded entry = %+v, ok=%v", e, ok)
	}
	if favs := s2.Favorites(); len(favs) != 1 || favs[0].Name != "Office PC" {
		t.Fatalf("Favorites() = %+v", favs)
	}
}

func TestEntriesWithoutNameOrFavoriteAreDropped(t *testing.T) {
	p := memPrefs{}
	s := Open(p)
	s.SetName(dev, "Office PC")
	s.SetName(dev, "")
	if _, ok := Open(p).Get(Key(dev.MAC, dev.IP)); ok {
		t.Fatal("entry with no name and no favorite was kept")
	}

	s.SetFavorite(dev, true)
	s.SetFavorite(dev, false)
	if _, ok := Open(p).Get(Key(dev.MAC, dev.IP)); ok {
		t.Fatal("unfavorited entry without a name was kept")
	}
}

func TestSeenUpdatesKnownEntriesOnly(t *testing.T) {
	p := memPrefs{}
	s := Open(p)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

	s.Seen(Entry{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.99"}, now)
	if _, ok := s.Get("aa:bb:cc:dd:ee:ff"); ok {
		t.Fatal("Seen created an entry for an unknown device")
	}

	s.SetFavorite(dev, true)
	moved := dev
	moved.IP = "192.168.1.40"
	s.Seen(moved, now)
	e, _ := Open(p).Get(Key(dev.MAC, ""))
	if e.IP != "192.168.1.40" || !e.LastSeen.Equal(now) {
		t.Fatalf("after Seen: %+v", e)
	}
}

func TestOpenIgnoresCorruptData(t *testing.T) {
	s := Open(memPrefs{prefsKey: "{not json"})
	if len(s.Favorites()) != 0 {
		t.Fatal("expected empty store")
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		10 * time.Second: "just now",
		5 * time.Minute:  "5m ago",
		3 * time.Hour:    "3h ago",
		50 * time.Hour:   "2d ago",
	}
	for d, want := range cases {
		if got := Ago(now.Add(-d), now); got != want {
			t.Errorf("Ago(-%v) = %q, want %q", d, got, want)
		}
	}
	if got := Ago(time.Time{}, now); got != "never" {
		t.Errorf("Ago(zero) = %q", got)
	}
}
