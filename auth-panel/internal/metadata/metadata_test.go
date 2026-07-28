package metadata

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDiscoverAudioFilesIgnoresTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Artist/song.mp3", "set/track.flac", "partial.mp3.part", ".hidden.ogg", "notes.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "outside.mp3"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-fileStabilityWindow - time.Second)
	for _, name := range []string{"Artist/song.mp3", "set/track.flac", "outside.mp3"} {
		if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "outside.mp3"), filepath.Join(dir, "linked.mp3")); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverAudioFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Artist/song.mp3", "outside.mp3", "set/track.flac"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverAudioFiles() = %v, want %v", got, want)
	}
}

func TestDiscoverAudioFilesSkipsRecentlyModifiedAudio(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "still-writing.mp3")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverAudioFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("DiscoverAudioFiles() = %v, want no files modified during the stability window", got)
	}
}

func TestFileObservationRequiresTwoUnchangedScans(t *testing.T) {
	seen := map[string]fileObservation{}
	first := fileObservation{size: 100, modified: time.Unix(100, 0)}
	if unchangedSincePreviousScan(seen, "track.mp3", first) {
		t.Fatal("first observation must not be processed")
	}
	if !unchangedSincePreviousScan(seen, "track.mp3", first) {
		t.Fatal("unchanged second observation must be processed")
	}
	if unchangedSincePreviousScan(seen, "track.mp3", fileObservation{size: 101, modified: first.modified}) {
		t.Fatal("changed observation must not be processed")
	}
}

func TestParseFilenameFallback(t *testing.T) {
	got := ParseFilename("Music/Artist Name - Album Name - 01 - Track Name.flac")
	want := Track{Artist: "Artist Name", Album: "Album Name", Title: "Track Name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFilename() = %#v, want %#v", got, want)
	}
}

func TestParseFilenameThreePartFallback(t *testing.T) {
	got := ParseFilename("Music/Artist Name - Album Name - Track Name.flac")
	want := Track{Artist: "Artist Name", Album: "Album Name", Title: "Track Name"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseFilename() = %#v, want %#v", got, want)
	}
}

func TestMergeTrackHintsUsesFilenameForMissingTags(t *testing.T) {
	got := mergeTrackHints(Track{Title: "Tagged Song"}, Track{Artist: "Artist Name", Album: "Album Name", Title: "Filename Song"})
	want := Track{Artist: "Artist Name", Album: "Album Name", Title: "Tagged Song"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeTrackHints() = %#v, want %#v", got, want)
	}
}

func TestHighConfidenceRequiresExactTitleAndArtist(t *testing.T) {
	tests := []struct {
		name  string
		input Track
		match Candidate
		want  bool
	}{
		{"exact normalized", Track{Title: "Jóga", Artist: "Björk"}, Candidate{Title: "Jóga", Artist: "Björk"}, true},
		{"title only is not safe", Track{Title: "Jóga"}, Candidate{Title: "Jóga", Artist: "Björk"}, false},
		{"different artist", Track{Title: "Jóga", Artist: "Björk"}, Candidate{Title: "Jóga", Artist: "Cover band"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHighConfidence(tt.input, tt.match); got != tt.want {
				t.Fatalf("IsHighConfidence() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyTagsWritesToTemporaryFileBeforeReplacingOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Artist - Song.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o640); err != nil {
		t.Fatal(err)
	}
	var gotName string
	var gotArgs []string
	oldRun := runFFmpeg
	oldChown := chownFile
	var gotOwnerPath string
	var gotUID, gotGID int
	runFFmpeg = func(_ context.Context, name string, args ...string) error {
		gotName, gotArgs = name, args
		return os.WriteFile(args[len(args)-1], []byte("retagged"), 0o640)
	}
	chownFile = func(name string, uid, gid int) error {
		gotOwnerPath, gotUID, gotGID = name, uid, gid
		return nil
	}
	t.Cleanup(func() { runFFmpeg = oldRun; chownFile = oldChown })

	if err := ApplyTags(context.Background(), path, Track{Title: "Song", Artist: "Artist", Album: "Album"}); err != nil {
		t.Fatal(err)
	}
	if gotName != "ffmpeg" {
		t.Fatalf("command = %q, want ffmpeg", gotName)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"-i " + path, "-metadata title=Song", "-metadata artist=Artist", "-metadata album=Album", "-c copy"} {
		if !strings.Contains(joined, want) {
			t.Errorf("ffmpeg arguments %q do not contain %q", joined, want)
		}
	}
	if gotArgs[len(gotArgs)-1] == path || !strings.Contains(filepath.Base(gotArgs[len(gotArgs)-1]), ".metadata-") {
		t.Fatalf("output %q is not a temporary metadata file", gotArgs[len(gotArgs)-1])
	}
	stat := mustStat(t, path)
	if gotOwnerPath != gotArgs[len(gotArgs)-1] || gotUID != int(stat.Uid) || gotGID != int(stat.Gid) {
		t.Fatalf("temporary ownership = %q %d:%d, want %q %d:%d", gotOwnerPath, gotUID, gotGID, gotArgs[len(gotArgs)-1], stat.Uid, stat.Gid)
	}
}

func mustStat(t *testing.T, path string) *syscall.Stat_t {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Sys().(*syscall.Stat_t)
}

func TestOwnershipOfReadsUnixFileOwner(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	uid, gid, err := ownershipOf(info)
	if err != nil {
		t.Fatal(err)
	}
	if uid != int(stat.Uid) || gid != int(stat.Gid) {
		t.Fatalf("ownershipOf() = %d:%d, want %d:%d", uid, gid, stat.Uid, stat.Gid)
	}
}
