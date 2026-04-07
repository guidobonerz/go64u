package streams

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"
)

// Matroska EBML element IDs
var (
	elEBML             = []byte{0x1A, 0x45, 0xDF, 0xA3}
	elEBMLVersion      = []byte{0x42, 0x86}
	elEBMLReadVersion  = []byte{0x42, 0xF7}
	elEBMLMaxIDLen     = []byte{0x42, 0xF2}
	elEBMLMaxSizeLen   = []byte{0x42, 0xF3}
	elDocType          = []byte{0x42, 0x82}
	elDocTypeVersion   = []byte{0x42, 0x87}
	elDocTypeReadVer   = []byte{0x42, 0x85}
	elSegment          = []byte{0x18, 0x53, 0x80, 0x67}
	elInfo             = []byte{0x15, 0x49, 0xA9, 0x66}
	elTimestampScale   = []byte{0x2A, 0xD7, 0xB1}
	elMuxingApp        = []byte{0x4D, 0x80}
	elWritingApp       = []byte{0x57, 0x41}
	elTracks           = []byte{0x16, 0x54, 0xAE, 0x6B}
	elTrackEntry       = []byte{0xAE}
	elTrackNumber      = []byte{0xD7}
	elTrackUID         = []byte{0x73, 0xC5}
	elTrackType        = []byte{0x83}
	elCodecID          = []byte{0x86}
	elCodecPrivate     = []byte{0x63, 0xA2}
	elDefaultDuration  = []byte{0x23, 0xE3, 0x83}
	elVideo            = []byte{0xE0}
	elPixelWidth       = []byte{0xB0}
	elPixelHeight      = []byte{0xBA}
	elAudio            = []byte{0xE1}
	elSamplingFreq     = []byte{0xB5}
	elChannels         = []byte{0x9F}
	elBitDepth         = []byte{0x62, 0x64}
	elCluster          = []byte{0x1F, 0x43, 0xB6, 0x75}
	elClusterTimestamp  = []byte{0xE7}
	elSimpleBlock      = []byte{0xA3}
)

// unknownSize is the EBML marker for unbounded streaming elements.
var unknownSize = []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// MkvMuxer writes interleaved audio and video into a Matroska container.
// It is safe for concurrent use from separate goroutines.
type MkvMuxer struct {
	w            io.Writer
	mu           sync.Mutex
	startTime    time.Time
	clusterTS    int64  // current cluster timestamp in ms
	audioBuf     []byte // audio accumulated between video frames
	audioEnabled bool
}

// NewMkvMuxer creates a Matroska muxer that writes to w.
// Track 1 = PNG video, Track 2 = PCM s16le stereo 48 kHz audio.
func NewMkvMuxer(w io.Writer, videoWidth, videoHeight, fps int, enableAudio bool) (*MkvMuxer, error) {
	m := &MkvMuxer{
		w:            w,
		startTime:    time.Now(),
		audioEnabled: enableAudio,
	}

	// EBML Header
	ebmlHeader := join(
		mkUint(elEBMLVersion, 1),
		mkUint(elEBMLReadVersion, 1),
		mkUint(elEBMLMaxIDLen, 4),
		mkUint(elEBMLMaxSizeLen, 8),
		mkString(elDocType, "matroska"),
		mkUint(elDocTypeVersion, 4),
		mkUint(elDocTypeReadVer, 2),
	)
	mkElement(m.w, elEBML, ebmlHeader)

	// Segment (unknown size — streaming)
	m.w.Write(elSegment)
	m.w.Write(unknownSize)

	// Info
	info := join(
		mkUint(elTimestampScale, 1000000), // 1 ms granularity
		mkString(elMuxingApp, "go64u"),
		mkString(elWritingApp, "go64u"),
	)
	mkElement(m.w, elInfo, info)

	// Tracks (audio track omitted when audioEnabled is false)
	trackData := [][]byte{buildVideoTrack(videoWidth, videoHeight, fps)}
	if m.audioEnabled {
		trackData = append(trackData, buildAudioTrack())
	}
	mkElement(m.w, elTracks, join(trackData...))

	// First Cluster at timestamp 0
	m.writeCluster(0)

	return m, nil
}

// WriteVideoFrame writes a PNG-encoded frame to the video track,
// then flushes buffered audio as properly-sized blocks.
func (m *MkvMuxer) WriteVideoFrame(pngData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ts := time.Since(m.startTime).Milliseconds()
	m.maybeNewCluster(ts)
	relTS := int16(ts - m.clusterTS)

	// Write video frame
	m.writeSimpleBlock(1, relTS, 0x80, pngData)

	// Flush buffered audio in fixed-size blocks.
	// At 48kHz stereo 16-bit, one frame duration (33ms) = 48000*2*2*33/1000 = 6336 bytes.
	// We write blocks of ~6144 bytes (1536 samples) to keep blocks manageable.
	if m.audioEnabled {
		m.flushAudio(relTS)
	}
}

// WriteAudio buffers raw PCM audio data until the next video frame flushes it.
func (m *MkvMuxer) WriteAudio(pcmData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audioBuf = append(m.audioBuf, pcmData...)
}

const audioBlockSize = 6144 // ~32ms of 48kHz stereo s16le (1536 samples * 4 bytes)

func (m *MkvMuxer) flushAudio(relTS int16) {
	for len(m.audioBuf) >= audioBlockSize {
		m.writeSimpleBlock(2, relTS, 0x80, m.audioBuf[:audioBlockSize])
		m.audioBuf = m.audioBuf[audioBlockSize:]
	}
	// Write any remaining partial block
	if len(m.audioBuf) > 0 {
		m.writeSimpleBlock(2, relTS, 0x80, m.audioBuf)
		m.audioBuf = m.audioBuf[:0]
	}
}

func (m *MkvMuxer) maybeNewCluster(ts int64) {
	// New cluster every 5 s to keep int16 relative timestamps in range
	if ts-m.clusterTS > 5000 {
		m.writeCluster(ts)
	}
}

func (m *MkvMuxer) writeCluster(ts int64) {
	m.clusterTS = ts
	m.w.Write(elCluster)
	m.w.Write(unknownSize)
	m.w.Write(mkUint(elClusterTimestamp, uint64(ts)))
}

func (m *MkvMuxer) writeSimpleBlock(trackNum int, relTS int16, flags byte, data []byte) {
	trackVINT := []byte{byte(trackNum) | 0x80} // works for track 1..126
	blockSize := len(trackVINT) + 3 + len(data) // VINT + ts(2) + flags(1) + data

	m.w.Write(elSimpleBlock)
	writeVINT(m.w, blockSize)
	m.w.Write(trackVINT)

	var ts [2]byte
	binary.BigEndian.PutUint16(ts[:], uint16(relTS))
	m.w.Write(ts[:])

	m.w.Write([]byte{flags})
	m.w.Write(data)
}

// --- Matroska track builders ------------------------------------------------

func buildVideoTrack(width, height, fps int) []byte {
	// BitmapInfoHeader (40 bytes) for V_MS/VFW/FOURCC with MPNG fourcc
	bih := make([]byte, 40)
	binary.LittleEndian.PutUint32(bih[0:], 40)
	binary.LittleEndian.PutUint32(bih[4:], uint32(width))
	binary.LittleEndian.PutUint32(bih[8:], uint32(height))
	binary.LittleEndian.PutUint16(bih[12:], 1)  // planes
	binary.LittleEndian.PutUint16(bih[14:], 24) // bits per pixel
	copy(bih[16:20], []byte("MPNG"))

	entry := join(
		mkUint(elTrackNumber, 1),
		mkUint(elTrackUID, 1),
		mkUint(elTrackType, 1), // video
		mkString(elCodecID, "V_MS/VFW/FOURCC"),
		mkBin(elCodecPrivate, bih),
		mkUint(elDefaultDuration, uint64(1000000000/fps)), // ns per frame
		mkElementBytes(elVideo, join(
			mkUint(elPixelWidth, uint64(width)),
			mkUint(elPixelHeight, uint64(height)),
		)),
	)
	return mkElementBytes(elTrackEntry, entry)
}

func buildAudioTrack() []byte {
	entry := join(
		mkUint(elTrackNumber, 2),
		mkUint(elTrackUID, 2),
		mkUint(elTrackType, 2), // audio
		mkString(elCodecID, "A_PCM/INT/LIT"),
		mkElementBytes(elAudio, join(
			mkFloat64(elSamplingFreq, 48000.0),
			mkUint(elChannels, 2),
			mkUint(elBitDepth, 16),
		)),
	)
	return mkElementBytes(elTrackEntry, entry)
}

// --- EBML encoding primitives -----------------------------------------------

// writeVINT writes a variable-length size to w.
// Each width reserves the all-ones pattern for "unknown size", so the
// max usable value per width is 2^(7*width) - 2.
func writeVINT(w io.Writer, size int) {
	switch {
	case size < 0x7F: // 1 byte: max 126 (0x7F = reserved)
		w.Write([]byte{byte(size) | 0x80})
	case size < 0x3FFF: // 2 bytes: max 16382 (0x3FFF = reserved)
		w.Write([]byte{byte(size>>8) | 0x40, byte(size)})
	case size < 0x1FFFFF: // 3 bytes: max 2097150
		w.Write([]byte{byte(size>>16) | 0x20, byte(size >> 8), byte(size)})
	default: // 4 bytes
		w.Write([]byte{byte(size>>24) | 0x10, byte(size >> 16), byte(size >> 8), byte(size)})
	}
}

// mkElement writes an EBML element (id + size + content) to w.
func mkElement(w io.Writer, id, content []byte) {
	w.Write(id)
	writeVINT(w, len(content))
	w.Write(content)
}

// mkElementBytes returns an EBML element as a byte slice (for nesting).
func mkElementBytes(id, content []byte) []byte {
	var buf bytes.Buffer
	mkElement(&buf, id, content)
	return buf.Bytes()
}

// mkUint returns an EBML unsigned integer element.
func mkUint(id []byte, val uint64) []byte {
	n := 1
	for v := val; v > 0xFF; v >>= 8 {
		n++
	}
	data := make([]byte, n)
	v := val
	for i := n - 1; i >= 0; i-- {
		data[i] = byte(v)
		v >>= 8
	}
	var buf bytes.Buffer
	buf.Write(id)
	writeVINT(&buf, n)
	buf.Write(data)
	return buf.Bytes()
}

// mkFloat64 returns an EBML 64-bit float element.
func mkFloat64(id []byte, val float64) []byte {
	var buf bytes.Buffer
	buf.Write(id)
	writeVINT(&buf, 8)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(val))
	buf.Write(b[:])
	return buf.Bytes()
}

// mkString returns an EBML UTF-8 string element.
func mkString(id []byte, val string) []byte {
	var buf bytes.Buffer
	buf.Write(id)
	writeVINT(&buf, len(val))
	buf.Write([]byte(val))
	return buf.Bytes()
}

// mkBin returns an EBML binary element.
func mkBin(id, val []byte) []byte {
	var buf bytes.Buffer
	buf.Write(id)
	writeVINT(&buf, len(val))
	buf.Write(val)
	return buf.Bytes()
}

// join concatenates byte slices.
func join(parts ...[]byte) []byte {
	var buf bytes.Buffer
	for _, p := range parts {
		buf.Write(p)
	}
	return buf.Bytes()
}
