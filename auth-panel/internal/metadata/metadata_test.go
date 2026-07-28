package metadata

import (
	"os"
	"path/filepath"
	"reflect"
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
