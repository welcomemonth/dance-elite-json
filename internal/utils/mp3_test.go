package utils

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

// mp3TestFrame 构造一个 MPEG1 Layer III、128kbps、44.1kHz 的静音帧
// （帧长 417 字节，1152 采样 ≈ 26.12ms）。
func mp3TestFrame() []byte {
	frame := make([]byte, 417)
	frame[0] = 0xff
	frame[1] = 0xfb // MPEG1, Layer III, no CRC
	frame[2] = 0x90 // bitrate index 9 = 128kbps, sample rate index 0 = 44100, no padding
	frame[3] = 0x64 // joint stereo
	return frame
}

// mp3TestFile 拼接 n 个帧。
func mp3TestFile(n int) []byte {
	frame := mp3TestFrame()
	out := make([]byte, 0, n*len(frame))
	for i := 0; i < n; i++ {
		out = append(out, frame...)
	}
	return out
}

func TestMP3DurationCBR(t *testing.T) {
	frames := 100
	data := mp3TestFile(frames)
	got, err := MP3Duration(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("MP3Duration: %v", err)
	}
	want := time.Duration(float64(frames*1152) / 44100 * float64(time.Second))
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	if diff > 5*time.Millisecond {
		t.Fatalf("duration = %v, want ≈ %v", got, want)
	}
}

func TestMP3DurationSkipsID3v2(t *testing.T) {
	tag := make([]byte, 10+20)
	copy(tag, "ID3")
	tag[3], tag[4] = 0x04, 0x00 // ID3v2.4
	tag[9] = 20                 // syncsafe size = 20
	data := append(tag, mp3TestFile(50)...)

	got, err := MP3Duration(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("MP3Duration with ID3: %v", err)
	}
	frames := 50
	want := time.Duration(float64(frames*1152) / 44100 * float64(time.Second))
	if got < want-5*time.Millisecond || got > want+5*time.Millisecond {
		t.Fatalf("duration = %v, want ≈ %v", got, want)
	}
}

func TestMP3DurationRejectsNonMP3(t *testing.T) {
	cases := map[string][]byte{
		"empty":       {},
		"text":        []byte("this is definitely not an mp3 file"),
		"png header":  []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3, 4},
		"broken sync": append([]byte{0xff, 0x00, 0x00, 0x00}, make([]byte, 100)...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := MP3Duration(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrInvalidMP3) {
				t.Fatalf("err = %v, want ErrInvalidMP3", err)
			}
		})
	}
}

func TestMP3DurationRejectsGarbageAfterFirstFrame(t *testing.T) {
	data := append(mp3TestFrame(), bytes.Repeat([]byte{0x00}, mp3MaxInvalidRun+100)...)
	if _, err := MP3Duration(bytes.NewReader(data), int64(len(data))); !errors.Is(err, ErrInvalidMP3) {
		t.Fatalf("err = %v, want ErrInvalidMP3", err)
	}
}

func TestMP3DurationFourMinuteBoundary(t *testing.T) {
	// 4 分钟 = 9187.5 帧；9190 帧应略超 4 分钟。
	data := mp3TestFile(9190)
	got, err := MP3Duration(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("MP3Duration: %v", err)
	}
	if got <= 4*time.Minute {
		t.Fatalf("duration = %v, want > 4m", got)
	}
}

// 确保测试文件大小计算使用的 Reader 满足 io.ReadSeeker。
var _ io.ReadSeeker = (*bytes.Reader)(nil)
