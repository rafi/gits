package cache

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/go-homedir"

	"github.com/rafi/gits/domain"
	"github.com/rafi/gits/internal/logging"
	"github.com/rafi/gits/internal/version"
)

func TestCacheFilePath(t *testing.T) {
	t.Run("honors XDG_CACHE_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", dir)
		got, err := cacheFilePath("mykey")
		if err != nil {
			t.Fatalf("cacheFilePath: %v", err)
		}
		want := filepath.Join(dir, "gits", "mykey.json")
		if got != want {
			t.Errorf("cacheFilePath = %q, want %q", got, want)
		}
	})

	t.Run("defaults to ~/.cache when unset", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", "")
		got, err := cacheFilePath("mykey")
		if err != nil {
			t.Fatalf("cacheFilePath: %v", err)
		}
		home, err := homedir.Dir()
		if err != nil {
			t.Fatalf("homedir.Dir: %v", err)
		}
		want := filepath.Join(home, ".cache", "gits", "mykey.json")
		if got != want {
			t.Errorf("cacheFilePath = %q, want %q", got, want)
		}
	})
}

// hashedProject returns a project with its checksum computed.
func hashedProject(t *testing.T) domain.Project {
	t.Helper()
	p := domain.Project{Name: "proj", Repos: []domain.Repository{{Name: "r"}}}
	if err := p.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}
	return p
}

// cacheTestKey is the fixed key used by the Get busting-matrix subtests.
const cacheTestKey = "k"

// writeCache writes a File as JSON to the cache path for cacheTestKey.
func writeCache(t *testing.T, f payload) {
	t.Helper()
	path, err := cacheFilePath(cacheTestKey)
	if err != nil {
		t.Fatalf("cacheFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestCacheFileGet(t *testing.T) {
	now := time.Now().Format(cacheTimeFormat)
	proj := hashedProject(t)

	t.Run("missing file", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		cf := &File{}
		got := proj
		ok, err := cf.Get("absent", &got)
		if err != nil || ok {
			t.Errorf("Get(missing) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("corrupt file is a miss, not an error", func(t *testing.T) {
		// A truncated or garbled cache file (interrupted write, disk full)
		// must degrade to a refresh — never a hard error that blocks every
		// command until the file is deleted by hand.
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		path, err := cacheFilePath(cacheTestKey)
		if err != nil {
			t.Fatalf("cacheFilePath: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(`{"version":"v0.12","proj`), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		cf := &File{ttl: time.Hour}
		got := proj
		ok, err := cf.Get(cacheTestKey, &got)
		if err != nil || ok {
			t.Errorf("Get(corrupt) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("version mismatch busts", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		writeCache(t, payload{Version: "v0.0", Timestamp: now, Checksum: proj.Hash, Project: proj})
		cf := &File{ttl: time.Hour}
		got := proj
		ok, err := cf.Get(cacheTestKey, &got)
		if err != nil || ok {
			t.Errorf("Get(version mismatch) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("checksum mismatch busts", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		writeCache(t, payload{Version: version.GetMajorMinor(), Timestamp: now, Checksum: "deadbeef", Project: proj})
		cf := &File{ttl: time.Hour}
		got := proj // got.Hash != "deadbeef"
		ok, err := cf.Get(cacheTestKey, &got)
		if err != nil || ok {
			t.Errorf("Get(checksum mismatch) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("expired via tiny ttl busts", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		writeCache(t, payload{Version: version.GetMajorMinor(), Timestamp: now, Checksum: proj.Hash, Project: proj})
		cf := &File{ttl: time.Nanosecond}
		got := proj
		ok, err := cf.Get(cacheTestKey, &got)
		if err != nil || ok {
			t.Errorf("Get(expired) = (%v, %v), want (false, nil)", ok, err)
		}
	})

	t.Run("valid hit via large ttl", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		writeCache(t, payload{Version: version.GetMajorMinor(), Timestamp: now, Checksum: proj.Hash, Project: proj})
		cf := &File{ttl: 1000 * time.Hour}
		got := domain.Project{Hash: proj.Hash}
		ok, err := cf.Get(cacheTestKey, &got)
		if err != nil || !ok {
			t.Fatalf("Get(valid) = (%v, %v), want (true, nil)", ok, err)
		}
		if got.Name != proj.Name {
			t.Errorf("populated project name = %q, want %q", got.Name, proj.Name)
		}
	})
}

func TestCacheFileSaveGetRoundTrip(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	proj := hashedProject(t)

	cf := &File{ttl: time.Hour}
	if err := cf.Save("rt", proj); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The write must be atomic: only the final file, no temp leftovers.
	entries, err := os.ReadDir(filepath.Join(cacheHome, "gits"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "rt.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("stray files after Save: %v", names)
	}

	reader := &File{ttl: time.Hour}
	got := domain.Project{Hash: proj.Hash}
	ok, err := reader.Get("rt", &got)
	if err != nil || !ok {
		t.Fatalf("Get after Save = (%v, %v), want (true, nil)", ok, err)
	}
	if got.Name != proj.Name {
		t.Errorf("round-trip name = %q, want %q", got.Name, proj.Name)
	}
}

//nolint:paralleltest // every subtest points XDG_CACHE_HOME at its own dir with t.Setenv.
func TestCacheFileFlush(t *testing.T) {
	t.Run("removes existing cache file", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		proj := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "rafi"},
		}
		key := proj.Source.UniqueKey()
		cf := &File{}
		if err := cf.Save(key, proj); err != nil {
			t.Fatalf("Save: %v", err)
		}
		path, _ := cacheFilePath(key)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected cache file to exist: %v", err)
		}
		if err := cf.Flush(proj); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("cache file still present after Flush: %v", err)
		}
	})

	t.Run("invalid source errors", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		proj := domain.Project{Source: &domain.ProviderSource{Type: "unknown"}}
		cf := &File{}
		if err := cf.Flush(proj); err == nil {
			t.Error("Flush with invalid source = nil, want error")
		}
	})

	t.Run("absent cache file is a no-op", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		proj := domain.Project{
			Source: &domain.ProviderSource{Type: "github", Search: "rafi"},
		}
		cf := &File{}
		if err := cf.Flush(proj); err != nil {
			t.Errorf("Flush with no cache file = %v, want nil", err)
		}
	})

	t.Run("nil source errors instead of panicking", func(t *testing.T) {
		cf := &File{}
		if err := cf.Flush(domain.Project{}); err == nil {
			t.Error("Flush with nil source = nil, want error")
		}
	})
}

// TestCacheFileZeroTTLAlwaysFresh proves an explicit cacheTTL of "0s" means
// "never serve from cache" rather than silently restoring the 7-day default.
func TestCacheFileZeroTTLAlwaysFresh(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	proj := hashedProject(t)
	writeCache(t, payload{
		Version:   version.GetMajorMinor(),
		Timestamp: time.Now().Format(cacheTimeFormat),
		Checksum:  proj.Hash,
		Project:   proj,
	})
	cf := &File{ttl: 0}
	got := proj
	ok, err := cf.Get(cacheTestKey, &got)
	if err != nil || ok {
		t.Errorf("Get(ttl=0) = (%v, %v), want (false, nil): zero ttl disables caching", ok, err)
	}
}

// TestCacheFileGetNoCrossTalk proves one client reading two cache files never
// leaks fields from the first file into the second read (the on-disk payload
// must not be unmarshalled into shared client state).
func TestCacheFileGetNoCrossTalk(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	first := domain.Project{Name: "first", Desc: "leaky description"}
	if err := first.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}
	second := domain.Project{Name: "second"} // no Desc
	if err := second.CalculateHash(); err != nil {
		t.Fatalf("CalculateHash: %v", err)
	}

	cf := &File{ttl: time.Hour}
	if err := cf.Save("first", first); err != nil {
		t.Fatalf("Save(first): %v", err)
	}
	if err := cf.Save("second", second); err != nil {
		t.Fatalf("Save(second): %v", err)
	}

	got1 := domain.Project{Hash: first.Hash}
	if ok, err := cf.Get("first", &got1); err != nil || !ok {
		t.Fatalf("Get(first) = (%v, %v), want hit", ok, err)
	}
	got2 := domain.Project{Hash: second.Hash}
	if ok, err := cf.Get("second", &got2); err != nil || !ok {
		t.Fatalf("Get(second) = (%v, %v), want hit", ok, err)
	}
	if got2.Desc != "" {
		t.Errorf("second project desc = %q, want empty (leaked from first read)", got2.Desc)
	}
}

// TestCacheFileTracesToItsLogger proves the cache reports its hits and misses
// to the logger it was constructed with, rather than to a global one — and
// that a zero-value File, which every other test here builds, traces to
// nothing instead of panicking.
func TestCacheFileTracesToItsLogger(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	var buf bytes.Buffer
	cf := NewFileCache(time.Hour, logging.New(&buf, true))

	proj := hashedProject(t)
	if ok, err := cf.Get("absent", &proj); ok || err != nil {
		t.Fatalf("Get(missing) = (%v, %v), want (false, nil)", ok, err)
	}
	if got := buf.String(); !strings.Contains(got, "cache file not found") {
		t.Errorf("trace = %q, want the miss recorded on the logger passed in", got)
	}

	// A File built without a logger must still be usable.
	buf.Reset()
	bare := &File{ttl: time.Hour}
	if ok, err := bare.Get("absent", &proj); ok || err != nil {
		t.Fatalf("zero-value Get(missing) = (%v, %v), want (false, nil)", ok, err)
	}
	if buf.Len() != 0 {
		t.Errorf("a File built without a logger wrote %q, want nothing", buf.String())
	}
}

func TestDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(dir, "gits")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}

	// Dir and the entries themselves must never name different directories.
	path, err := cacheFilePath("anykey")
	if err != nil {
		t.Fatalf("cacheFilePath: %v", err)
	}
	if filepath.Dir(path) != got {
		t.Errorf("Dir() = %q, but entries go in %q", got, filepath.Dir(path))
	}
}

// writeCacheKeyed writes a cache file under an arbitrary key, so Entries can
// be given several at once.
func writeCacheKeyed(t *testing.T, key string, f payload) {
	t.Helper()
	path, err := cacheFilePath(key)
	if err != nil {
		t.Fatalf("cacheFilePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestEntries(t *testing.T) {
	ttl := time.Hour

	// A missing cache directory means nothing has been cached yet, which is
	// not a failure: `gits doctor` reports it as an absence.
	t.Run("missing directory is not an error", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "nothing-here"))

		entries, err := Entries(ttl)
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(entries) != 0 {
			t.Errorf("Entries() = %d entries, want 0", len(entries))
		}
	})

	t.Run("describes each entry and sorts by key", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		live := version.GetMajorMinor()
		now := time.Now().Format(cacheTimeFormat)
		old := time.Now().Add(-2 * time.Hour).Format(cacheTimeFormat)

		writeCacheKeyed(t, "zzz-live", payload{Version: live, Timestamp: now})
		writeCacheKeyed(t, "aaa-expired", payload{Version: live, Timestamp: old})
		writeCacheKeyed(t, "mmm-oldversion", payload{Version: "v0.0", Timestamp: now})

		// Neither of these is a cache entry: one is not JSON-suffixed, the
		// other is a directory.
		dir, err := Dir()
		if err != nil {
			t.Fatalf("Dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "sub.json"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		entries, err := Entries(ttl)
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("Entries() = %d entries, want 3 (%+v)", len(entries), entries)
		}

		wantKeys := []string{"aaa-expired", "mmm-oldversion", "zzz-live"}
		for i, want := range wantKeys {
			if entries[i].Key != want {
				t.Errorf("entries[%d].Key = %q, want %q — entries sort by key", i, entries[i].Key, want)
			}
		}
		if got := entries[0].Unusable; got != "expired" {
			t.Errorf("expired entry Unusable = %q, want %q", got, "expired")
		}
		// A version mismatch busts an entry regardless of its age, so it is
		// reported ahead of expiry.
		if got := entries[1].Unusable; !strings.Contains(got, "v0.0") {
			t.Errorf("old-version entry Unusable = %q, want it to name v0.0", got)
		}
		if got := entries[2].Unusable; got != "" {
			t.Errorf("live entry Unusable = %q, want empty", got)
		}
		if entries[2].Version != live {
			t.Errorf("live entry Version = %q, want %q", entries[2].Version, live)
		}
		if entries[2].CachedAt.IsZero() {
			t.Error("live entry CachedAt is zero, want the cached timestamp")
		}
	})

	t.Run("a corrupt file is reported, not fatal", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		path, err := cacheFilePath("broken")
		if err != nil {
			t.Fatalf("cacheFilePath: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}

		entries, err := Entries(ttl)
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("Entries() = %d entries, want 1", len(entries))
		}
		if entries[0].Unusable != "corrupt" {
			t.Errorf("Unusable = %q, want %q", entries[0].Unusable, "corrupt")
		}
	})

	t.Run("an unparsable timestamp is reported", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())
		writeCacheKeyed(t, "notime", payload{Version: version.GetMajorMinor(), Timestamp: "whenever"})

		entries, err := Entries(ttl)
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("Entries() = %d entries, want 1", len(entries))
		}
		if entries[0].Unusable != "unreadable timestamp" {
			t.Errorf("Unusable = %q, want %q", entries[0].Unusable, "unreadable timestamp")
		}
	})
}
