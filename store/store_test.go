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

func TestLearningMACMergesIPEntry(t *testing.T) {
	p := memPrefs{}
	s := Open(p)
	byIP := Entry{IP: dev.IP, Hostname: dev.Hostname}
	s.SetFavorite(byIP, true)
	s.SetName(byIP, "Office PC")

	s.SetFavorite(dev, true)

	favs := Open(p).Favorites()
	if len(favs) != 1 || favs[0].Name != "Office PC" || favs[0].MAC != "30:52:53:00:1a:2b" {
		t.Fatalf("Favorites() = %+v", favs)
	}
	s.SetFavorite(dev, false)
	s.SetName(dev, "")
	if favs := Open(p).Favorites(); len(favs) != 0 {
		t.Fatalf("after unstarring, Favorites() = %+v", favs)
	}
}

func TestSeenWithMACMergesIPEntry(t *testing.T) {
	p := memPrefs{}
	s := Open(p)
	s.SetFavorite(Entry{IP: dev.IP}, true)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	s.Seen(dev, at)

	e, ok := Open(p).Get(Key(dev.MAC, dev.IP))
	if !ok || !e.Favorite || !e.LastSeen.Equal(at) {
		t.Fatalf("entry = %+v, ok=%v", e, ok)
	}
	if _, ok := Open(p).Get(Key("", dev.IP)); ok {
		t.Fatal("ip-keyed entry was kept")
	}
}

func TestOpenMergesSavedDuplicates(t *testing.T) {
	p := memPrefs{prefsKey: `[{"ip":"192.168.1.21","name":"Office PC","favorite":true},
		{"mac":"30:52:53:00:1a:2b","ip":"192.168.1.21","favorite":true}]`}
	favs := Open(p).Favorites()
	if len(favs) != 1 || favs[0].Name != "Office PC" || favs[0].MAC == "" {
		t.Fatalf("Favorites() = %+v", favs)
	}
}

func TestMACEntryAtOtherIPIsNotMerged(t *testing.T) {
	s := Open(memPrefs{})
	s.SetFavorite(Entry{IP: "192.168.1.30"}, true)
	s.SetFavorite(dev, true)
	if favs := s.Favorites(); len(favs) != 2 {
		t.Fatalf("Favorites() = %+v", favs)
	}
}
