// Package media provides core types and utilities for media stream identification,
// format specification, and metadata handling. It supports both multiplexed (combined)
// and track-based (separate) media representations, including live and on-demand sources.
package media

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"sync"

	"github.com/rebeljah/picast/util/fileutil"
	"gopkg.in/vansante/go-ffprobe.v2"
)

type ErrNoSuchID struct {
	id string
}

func (e ErrNoSuchID) Error() string { return fmt.Sprintf("no such id: %s", e.id) }

var errNoSuchID = ErrNoSuchID{}

// UID represents a unique identifier for an entire standalone or multiplexed
// media, like a movie song or live-stream
type UID string

// NewUID generates a cryptographically secure random ContentID using a
// 62-character alphanumeric charset. Returns an error if cryptographic
// randomness is unavailable.
func NewUID() (UID, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		randIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[randIndex.Int64()]
	}
	return UID(b), nil
}

// BasicMediaType defines the fundamental classification of media content.
type BasicMediaType string

const (
	AudioVideo      BasicMediaType = "av" // Combined audio and video streams
	StandaloneAudio BasicMediaType = "a"  // Audio-only content
	StandaloneVideo BasicMediaType = "v"  // Video-only content
)

// TrackRole defines the functional purpose and requirements of a media track.
type TrackRole string

const (
	RequiredTrackAudioRole   TrackRole = "requiredAudio"        // Mandatory audio track
	RequiredTrackVideoRole   TrackRole = "requiredVideo"        // Mandatory video track
	OptionalTrackAudioRole   TrackRole = "optionalAudio"        // Supplementary audio track
	StandaloneAudioRole      TrackRole = "standaloneAudio"      // Primary audio in audio-only content
	StandaloneVideoRole      TrackRole = "standaloneVideo"      // Primary video in video-only content
	MultiplexedContainerRole TrackRole = "multiplexedContainer" // container with multiple elements, usually a/v
)

// CodingFormat specifies the encoding standard used for media compression.
type CodingFormat string

const (
	AVC    CodingFormat = "AVC"
	AAC    CodingFormat = "AAC"
	MPEGTS CodingFormat = "MPEG-TS"
)

// TrackID is a human-readable, url-safe identifier for a media track
// (e.g., "main-audio", "commentary", "camera-angle-2").
type TrackID string

// TrackInfo describes a single track within a media,
// including its identifier, functional role, and technical encoding.
type TrackInfo struct {
	// should be used to distinguish between tracks in a media
	ID TrackID `sdp:"id" json:"id"`

	// Role defines the track's purpose and requirements
	Role TrackRole `sdp:"track-role" json:"trackRole"`

	// provides metadata about codec, bitrate, etc
	Spec *ffprobe.Stream

	MultiplexedElements []TrackInfo `sdp:"multiplexed-elements" json:"multiplexedElements"`
}

// StructureInfo describes the technical structure of a media,
// including its multiplexing format and track composition.
type StructureInfo struct {
	// BasicContentType classifies the media as AV, audio-only, or video-only
	BasicContentType BasicMediaType

	// Tracks describes all media tracks, mapped by their identifiers
	Tracks map[TrackID]TrackInfo `sdp:"tracks" json:"tracks"`
}

func NewStructureInfo() StructureInfo {
	return StructureInfo{
		Tracks: make(map[TrackID]TrackInfo),
	}
}

type Metadata struct {
	Title        string         `sdp:"title" json:"title"`                // Human-readable title
	UID          UID            `sdp:"id" json:"id"`                      // Unique content identifier
	MediaType    BasicMediaType `sdp:"media-type" json:"mediaType"`       // Content classification
	Genre        string         `sdp:"genre" json:"genre"`                // Content category
	Duration     float64        `sdp:"duration" json:"duration"`          // Runtime in seconds
	ThumbnailURL string         `sdp:"thumbnail-url" json:"thumbnailURL"` // Preview image URL
	IsLive       bool           `sdp:"is-live" json:"isLive"`
	Structure    StructureInfo  `sdp:"structure" json:"structure"`
}

func NewMetaData() Metadata {
	return Metadata{
		Structure: NewStructureInfo(),
	}
}

// LoadMetaDataFromJSON decodes JSON media metadata from an io.Reader.
// Automatically generates a ContentID if none is present in the input.
// Returns an error for invalid JSON or ID generation failures.
func LoadMetaDataFromJSON(r io.Reader) (Metadata, error) {
	var meta Metadata
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&meta); err != nil {
		return Metadata{}, err
	}

	if meta.UID == "" {
		newID, err := NewUID()
		if err != nil {
			return Metadata{}, err
		}
		meta.UID = newID
	}

	return meta, nil
}

// WriteJSON serializes the MetaData to JSON format and writes it to
// the provided io.Writer with human-readable indentation.
func (m Metadata) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m)
}

// WriteJSONToFile writes the MetaData to a filesystem path as formatted JSON.
// Creates or truncates the target file. Returns any filesystem operation errors.
func (m Metadata) WriteJSONToFile(name string) error {
	file, err := os.Create(name)
	if err != nil {
		return err
	}
	defer file.Close()

	return m.WriteJSON(file)
}

type Manifest interface {
	Get(uid UID) (Metadata, bool)
	JSON() ([]byte, error)
	SaveJSON(path string) error
	ContainsUID(id UID) bool
}

type MutableManifest interface {
	Manifest

	// Puts data into the manifest.
	//  - if the new data id matches an existing data then the existing data is overwritten.
	Put(m Metadata)

	// Replaces all or part of the contents of one metadata with another.
	//   - patch data must include a non-zero ID so that it can be matched to an existing data
	//     in the manifest.
	//   - All other fields of the patch data may be left zero; only non-zero field will be copied.
	Patch(m Metadata) error

	// Zeroes fields in a data given a mask data.
	//  -  mask data must include a non-zero ID so that it can be matched to an existing data
	//     in the manifest.
	//  - All other non-zero fields of the mask will be zeroed in the existing data in the manifest.
	CutPatch(mask Metadata) error

	// Deletes the data with the given UID.
	//  - returns true iff the uid existed.
	//  - returns false iff the uid does not exist
	Delete(uid UID) bool
}

// implements MutableManifest
//   - use a sync.RWMutex to permit multiple readers, OR one writer, access concurrently
//     i.e all reads must finish before any write, and no write can begin while a read is occurring.
type FileManifest struct {
	lock     sync.RWMutex
	metadata map[UID]Metadata `json:"metadata"`
}

func NewFileManifest() MutableManifest {
	return &FileManifest{
		metadata: make(map[UID]Metadata),
	}
}

func NewFileManifestFromJSON(r io.Reader) (MutableManifest, error) {
	var manifest FileManifest
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&manifest); err != nil {
		return &FileManifest{}, err
	}
	return &manifest, nil
}

// JSON serializes the ContentManifest to a formatted JSON string.
// Returns an error for JSON marshaling failures.
func (m *FileManifest) JSON() ([]byte, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	manifestJSON, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Printf("Failed to serialize manifest: %v\n", err)
		return nil, err
	}
	return manifestJSON, nil
}

func (m *FileManifest) SaveJSON(path string) error {
	m.lock.RLock()
	defer m.lock.RUnlock()

	buf, err := m.JSON()
	if err != nil {
		return err
	}

	err = fileutil.ReplaceFileContents(path, buf)
	if err != nil {
		log.Printf("Failed to update manifest: %v\n", err)
	}

	return nil
}

func (m *FileManifest) Get(id UID) (Metadata, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	md, ok := m.metadata[id]
	return md, ok
}

func (m *FileManifest) ContainsUID(id UID) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()

	_, ok := m.metadata[id]
	return ok
}

func (m *FileManifest) Put(d Metadata) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.metadata[d.UID] = d
}

func (m *FileManifest) Patch(patch Metadata) error {
	m.lock.RLock()
	defer m.lock.Unlock()

	// old, ok := m.metadata[patch.UID]

	m.lock.RUnlock()

	// if !ok {
	// 	return ErrNoSuchID{id: string(patch.UID)}
	// }

	// TODO copy non-zero fields of patch to old (or visa-versa), then reinsert into m.metadata

	return nil
}

func (m *FileManifest) CutPatch(mask Metadata) error {
	return nil // TODO
}

func (m *FileManifest) Delete(uid UID) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	if !m.ContainsUID(uid) {
		return false
	}

	return false // TODO
}

// Source represents a readable content source with resource cleanup capability.
type Source interface {
	io.Reader
	io.Closer
}

// OnDemandSource extends Source with random access capability for non-linear playback.
type OnDemandSource interface {
	Source
	io.Seeker
}

type LiveSource interface {
	Source
}

// OnDemandFileSource implements OnDemandSource using a filesystem handle.
type OnDemandFileSource struct {
	mediaFile *os.File
}

// LoadOnDemandFileSource opens a filesystem path for random-access media reading.
// Returns an error if the file cannot be opened.
func LoadOnDemandFileSource(name string) (*OnDemandFileSource, error) {
	file, err := os.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &OnDemandFileSource{mediaFile: file}, nil
}

func (s *OnDemandFileSource) Read(p []byte) (int, error) {
	return s.mediaFile.Read(p)
}

func (s *OnDemandFileSource) Close() error {
	return s.mediaFile.Close()
}

func (s *OnDemandFileSource) Seek(offset int64, whence int) (int64, error) {
	return s.mediaFile.Seek(offset, whence)
}

// LiveFileSource implements LiveSource for sequential streaming from pipes.
type LiveFileSource struct {
	mediaPipe *os.File
}

// LoadLiveFileSource opens a named pipe for sequential media streaming.
// Returns an error if the pipe cannot be accessed.
func LoadLiveFileSource(name string) (*LiveFileSource, error) {
	pipe, err := os.OpenFile(name, os.O_RDONLY, os.ModeNamedPipe)
	if err != nil {
		return nil, err
	}
	return &LiveFileSource{mediaPipe: pipe}, nil
}

func (s *LiveFileSource) Read(p []byte) (int, error) {
	return s.mediaPipe.Read(p)
}

func (s *LiveFileSource) Close() error {
	return s.mediaPipe.Close()
}
