package local

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
)

// decoder is one open audio file as interleaved float32 stereo frames at
// its own sample rate: mono is copied to both channels and only the first
// two channels of a multichannel file are kept.
type decoder interface {
	rate() int
	// read fills dst (len(dst)/2 frames at most) and returns the frames
	// read: more than zero, or zero with an error, io.EOF at the end.
	read(dst []float32) (int, error)
	// length is the number of frames; 0 when unknown.
	length() int64
	// seek moves to the frame, counted from the start.
	seek(frame int64) error
	close() error
}

// openDecoder opens the file at path by its extension.
func openDecoder(path string) (decoder, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var d decoder
	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".wav":
		d, err = newWAV(f)
	case ".mp3":
		d, err = newMP3(f)
	case ".flac":
		d, err = newFLAC(f)
	case ".ogg", ".oga":
		d, err = newVorbis(f)
	default:
		err = fmt.Errorf("unsupported format %q", ext)
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("local: %s: %w", filepath.Base(path), err)
	}
	return d, nil
}

// toStereo writes frames frames of interleaved src with ch channels into
// dst as stereo.
func toStereo(dst, src []float32, ch, frames int) {
	for i := range frames {
		l := src[i*ch]
		r := l
		if ch > 1 {
			r = src[i*ch+1]
		}
		dst[2*i], dst[2*i+1] = l, r
	}
}

// wavDecoder reads uncompressed RIFF WAVE: integer PCM of 8, 16, 24 or 32
// bits and IEEE float of 32 or 64 bits, plain or WAVE_FORMAT_EXTENSIBLE.
type wavDecoder struct {
	f       *os.File
	sr, ch  int
	bits    int
	float   bool
	block   int
	data    int64 // offset of the samples
	frames  int64
	pos     int64 // next frame
	buf     []byte
	samples []float32
}

func newWAV(f *os.File) (*wavDecoder, error) {
	var riff [12]byte
	if _, err := io.ReadFull(f, riff[:]); err != nil || string(riff[:4]) != "RIFF" || string(riff[8:]) != "WAVE" {
		return nil, errors.New("not a RIFF WAVE file")
	}
	d := &wavDecoder{f: f}
	off := int64(12)
	haveFmt := false
	for {
		var hdr [8]byte
		if _, err := f.ReadAt(hdr[:], off); err != nil {
			return nil, errors.New("wav: no data chunk")
		}
		size := int64(binary.LittleEndian.Uint32(hdr[4:]))
		body := off + 8
		switch string(hdr[:4]) {
		case "fmt ":
			b := make([]byte, min(size, 40))
			if _, err := f.ReadAt(b, body); err != nil || len(b) < 16 {
				return nil, errors.New("wav: short fmt chunk")
			}
			code := binary.LittleEndian.Uint16(b)
			d.ch, d.sr = int(binary.LittleEndian.Uint16(b[2:])), int(binary.LittleEndian.Uint32(b[4:]))
			d.block, d.bits = int(binary.LittleEndian.Uint16(b[12:])), int(binary.LittleEndian.Uint16(b[14:]))
			if code == 0xfffe && len(b) >= 26 {
				code = binary.LittleEndian.Uint16(b[24:]) // the subformat GUID's first bytes
			}
			d.float = code == 3
			okInt := code == 1 && (d.bits == 8 || d.bits == 16 || d.bits == 24 || d.bits == 32)
			okFloat := d.float && (d.bits == 32 || d.bits == 64)
			if !okInt && !okFloat || d.ch < 1 || d.sr < 1 || d.block != d.ch*d.bits/8 {
				return nil, fmt.Errorf("wav: unsupported encoding (format %d, %d bits)", code, d.bits)
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, errors.New("wav: data before fmt")
			}
			d.data = body
			if info, err := f.Stat(); err == nil && body+size > info.Size() {
				size = info.Size() - body // a truncated file, or a streaming writer's size
			}
			d.frames = size / int64(d.block)
			return d, d.seek(0)
		}
		off = body + size + size&1
	}
}

func (d *wavDecoder) rate() int     { return d.sr }
func (d *wavDecoder) length() int64 { return d.frames }
func (d *wavDecoder) close() error  { return d.f.Close() }

func (d *wavDecoder) seek(frame int64) error {
	d.pos = min(max(frame, 0), d.frames)
	_, err := d.f.Seek(d.data+d.pos*int64(d.block), io.SeekStart)
	return err
}

func (d *wavDecoder) read(dst []float32) (int, error) {
	n := min(int64(len(dst)/2), d.frames-d.pos)
	if n <= 0 {
		return 0, io.EOF
	}
	need := int(n) * d.block
	if cap(d.buf) < need {
		d.buf = make([]byte, need)
	}
	got, err := io.ReadFull(d.f, d.buf[:need])
	frames := got / d.block
	if frames == 0 {
		if err == nil || errors.Is(err, io.ErrUnexpectedEOF) {
			err = io.EOF
		}
		return 0, err
	}
	if cap(d.samples) < frames*d.ch {
		d.samples = make([]float32, frames*d.ch)
	}
	s, b := d.samples[:frames*d.ch], d.buf
	width := d.bits / 8
	for i := range s {
		p := b[i*width:]
		switch {
		case d.float && width == 4:
			s[i] = math.Float32frombits(binary.LittleEndian.Uint32(p))
		case d.float:
			s[i] = float32(math.Float64frombits(binary.LittleEndian.Uint64(p)))
		case width == 1:
			s[i] = float32(int(p[0])-128) / 128
		case width == 2:
			s[i] = float32(int16(binary.LittleEndian.Uint16(p))) / 32768
		case width == 3:
			s[i] = float32(int32(uint32(p[0])<<8|uint32(p[1])<<16|uint32(p[2])<<24)>>8) / 8388608
		default:
			s[i] = float32(float64(int32(binary.LittleEndian.Uint32(p))) / 2147483648)
		}
	}
	toStereo(dst, s, d.ch, frames)
	d.pos += int64(frames)
	return frames, nil
}

// mp3Decoder wraps go-mp3, which always yields 16-bit stereo. Opening it
// scans every frame header (for Length and Seek).
type mp3Decoder struct {
	f   *os.File
	dec *mp3.Decoder
	buf []byte
}

func newMP3(f *os.File) (*mp3Decoder, error) {
	dec, err := mp3.NewDecoder(f)
	if err != nil {
		return nil, err
	}
	return &mp3Decoder{f: f, dec: dec}, nil
}

func (d *mp3Decoder) rate() int    { return d.dec.SampleRate() }
func (d *mp3Decoder) close() error { return d.f.Close() }

func (d *mp3Decoder) length() int64 {
	if l := d.dec.Length(); l > 0 {
		return l / 4
	}
	return 0
}

func (d *mp3Decoder) seek(frame int64) error {
	_, err := d.dec.Seek(max(frame, 0)*4, io.SeekStart)
	return err
}

func (d *mp3Decoder) read(dst []float32) (int, error) {
	need := len(dst) / 2 * 4
	if need == 0 {
		return 0, nil
	}
	if cap(d.buf) < need {
		d.buf = make([]byte, need)
	}
	got, err := io.ReadAtLeast(d.dec, d.buf[:need], 4)
	frames := got / 4
	for i := range frames * 2 {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(d.buf[2*i:]))) / 32768
	}
	if frames > 0 {
		return frames, nil
	}
	if err == nil || errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	return 0, err
}

// flacDecoder wraps mewkiz/flac, a frame (a block of samples) at a time.
type flacDecoder struct {
	f      *os.File
	stream *flac.Stream
	cur    *frame.Frame
	at     int   // next sample of cur
	skip   int64 // samples to drop after a seek landed before its target
	scale  float32
}

func newFLAC(f *os.File) (*flacDecoder, error) {
	stream, err := flac.NewSeek(f)
	if err != nil {
		return nil, err
	}
	return &flacDecoder{f: f, stream: stream, scale: 1 / float32(int64(1)<<(stream.Info.BitsPerSample-1))}, nil
}

func (d *flacDecoder) rate() int     { return int(d.stream.Info.SampleRate) }
func (d *flacDecoder) length() int64 { return int64(d.stream.Info.NSamples) }
func (d *flacDecoder) close() error  { return d.f.Close() }

// seek lands on the frame holding the target and drops the samples before
// it on the next reads.
func (d *flacDecoder) seek(target int64) error {
	target = max(target, 0)
	if n := d.length(); n > 0 && target >= n {
		target = n - 1
	}
	start, err := d.stream.Seek(uint64(target))
	if err != nil {
		return err
	}
	d.cur, d.at, d.skip = nil, 0, target-int64(start)
	return nil
}

func (d *flacDecoder) read(dst []float32) (int, error) {
	frames := 0
	for frames < len(dst)/2 {
		if d.cur == nil || d.at >= int(d.cur.BlockSize) {
			fr, err := d.stream.ParseNext()
			if err != nil {
				if frames > 0 {
					return frames, nil
				}
				return 0, err // io.EOF at the end
			}
			d.cur, d.at = fr, 0
			if d.skip > 0 {
				drop := min(d.skip, int64(fr.BlockSize))
				d.at, d.skip = int(drop), d.skip-drop
				continue
			}
		}
		sub := d.cur.Subframes
		for ; d.at < int(d.cur.BlockSize) && frames < len(dst)/2; d.at, frames = d.at+1, frames+1 {
			l := float32(sub[0].Samples[d.at]) * d.scale
			r := l
			if len(sub) > 1 {
				r = float32(sub[1].Samples[d.at]) * d.scale
			}
			dst[2*frames], dst[2*frames+1] = l, r
		}
	}
	return frames, nil
}

// vorbisDecoder wraps jfreymuth/oggvorbis.
type vorbisDecoder struct {
	f   *os.File
	r   *oggvorbis.Reader
	buf []float32
}

func newVorbis(f *os.File) (*vorbisDecoder, error) {
	r, err := oggvorbis.NewReader(f)
	if err != nil {
		return nil, err
	}
	return &vorbisDecoder{f: f, r: r}, nil
}

func (d *vorbisDecoder) rate() int              { return d.r.SampleRate() }
func (d *vorbisDecoder) length() int64          { return d.r.Length() }
func (d *vorbisDecoder) close() error           { return d.f.Close() }
func (d *vorbisDecoder) seek(frame int64) error { return d.r.SetPosition(max(frame, 0)) }

func (d *vorbisDecoder) read(dst []float32) (int, error) {
	ch := d.r.Channels()
	need := len(dst) / 2 * ch
	if cap(d.buf) < need {
		d.buf = make([]float32, need)
	}
	for {
		n, err := d.r.Read(d.buf[:need])
		if frames := n / ch; frames > 0 {
			toStereo(dst, d.buf, ch, frames)
			return frames, nil
		}
		if err != nil {
			return 0, err
		}
		if need == 0 {
			return 0, nil
		}
	}
}
