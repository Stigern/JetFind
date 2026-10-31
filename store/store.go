// Package store remembers favourite JetKVMs and custom names between runs.
// Devices are keyed by MAC address so they survive DHCP handing out a new IP.
package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const prefsKey = "devices"

// Backend is the subset of fyne.Preferences the store needs.
type Backend interface {
	String(key string) string
	SetString(key, value string)
}

// Entry is what we remember about one device.
type Entry struct {
	MAC      string    `json:"mac,omitempty"`
	IP       string    `json:"ip"`
	Hostname string    `json:"hostname,omitempty"`
	Name     string    `json:"name,omitempty"`
	Favorite bool      `json:"favorite,omitempty"`
	LastSeen time.Time `json:"lastSeen,omitzero"`
}

// Key identifies a device: its MAC when known, otherwise its IP.
func Key(mac, ip string) string {
	if mac != "" {
		return strings.ToLower(mac)
	}
	return "ip:" + ip
}

// Store holds entries that are a favourite or have a custom name.
type Store struct {
	b       Backend
	entries map[string]Entry
}

// Open loads saved entries; unreadable data is ignored.
func Open(b Backend) *Store {
	s := &Store{b: b, entries: map[string]Entry{}}
	var list []Entry
	if json.Unmarshal([]byte(b.String(prefsKey)), &list) == nil {
		for _, e := range list {
			s.entries[Key(e.MAC, e.IP)] = e
		}
		for _, e := range list {
			s.merge(e)
		}
	}
	return s
}

// Get returns the saved entry for key.
func (s *Store) Get(key string) (Entry, bool) {
	e, ok := s.entries[key]
	return e, ok
}

// Favorites returns all favourite entries.
func (s *Store) Favorites() []Entry {
	var out []Entry
	for _, e := range s.entries {
		if e.Favorite {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

// SetFavorite marks or unmarks d as a favourite.
func (s *Store) SetFavorite(d Entry, fav bool) {
	s.update(d, func(e *Entry) { e.Favorite = fav })
}

// SetName sets a custom name for d; an empty name removes it.
func (s *Store) SetName(d Entry, name string) {
	s.update(d, func(e *Entry) { e.Name = strings.TrimSpace(name) })
}

// Adopt moves anything saved for d by IP to its MAC, now that the MAC is known.
func (s *Store) Adopt(d Entry) {
	if s.merge(d) {
		s.save()
	}
}

// merge folds the ip-keyed entry for d.IP into the entry for d.MAC and
// reports whether there was one.
func (s *Store) merge(d Entry) bool {
	old, ok := s.entries[Key("", d.IP)]
	if d.MAC == "" || !ok {
		return false
	}
	delete(s.entries, Key("", d.IP))
	key := Key(d.MAC, d.IP)
	e := s.entries[key]
	e.MAC, e.IP = key, d.IP
	if e.Hostname == "" {
		e.Hostname = old.Hostname
	}
	if e.Name == "" {
		e.Name = old.Name
	}
	e.Favorite = e.Favorite || old.Favorite
	if old.LastSeen.After(e.LastSeen) {
		e.LastSeen = old.LastSeen
	}
	s.entries[key] = e
	return true
}

// Seen records that d was found at d.IP. Unknown devices are not stored.
func (s *Store) Seen(d Entry, at time.Time) {
	s.merge(d)
	key := Key(d.MAC, d.IP)
	if _, ok := s.entries[key]; !ok {
		return
	}
	s.update(d, func(e *Entry) { e.LastSeen = at })
}

// update applies f to the entry for d, refreshing its address details, and
// saves. Entries that end up neither favourite nor named are forgotten.
func (s *Store) update(d Entry, f func(*Entry)) {
	s.merge(d)
	key := Key(d.MAC, d.IP)
	e := s.entries[key]
	e.MAC = strings.ToLower(d.MAC)
	if d.IP != "" {
		e.IP = d.IP
	}
	if d.Hostname != "" {
		e.Hostname = d.Hostname
	}
	f(&e)
	if e.Favorite || e.Name != "" {
		s.entries[key] = e
	} else {
		delete(s.entries, key)
	}
	s.save()
}

func (s *Store) save() {
	list := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}
	sort.Slice(list, func(i, j int) bool { return Key(list[i].MAC, list[i].IP) < Key(list[j].MAC, list[j].IP) })
	b, _ := json.Marshal(list)
	s.b.SetString(prefsKey, string(b))
}

// Ago formats how long before now t was, e.g. "5m ago".
func Ago(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
