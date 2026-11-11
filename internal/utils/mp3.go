package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrInvalidMP3 表示文件不是可解析的 MP3（MPEG Audio Layer III）数据。
// 事务计划 R5 定稿：音频时长校验用帧头解析，解析失败一律拒绝并要求重传，
// 不做转码探测。
var ErrInvalidMP3 = errors.New("invalid mp3")

const (
	// mp3MaxInvalidRun 是连续无法识别为帧头的最大字节数。正常 MP3 的
	// ID3v1 尾部只有 128 字节；超过该阈值视为文件损坏，提前失败，
	// 避免对伪装成大文件（≤100MB）的非音频数据做全量逐字节扫描。
	mp3MaxInvalidRun = 4096

	// mp3ScanBufferSize 是帧头扫描的读取缓冲（帧负载通过 Discard 跳过）。
	mp3ScanBufferSize = 64 << 10
)

// mp3Frame 是一帧 Layer III 音频的长度与时长信息。
type mp3Frame struct {
	length     int
	samples    int
	sampleRate int
}

// MP3Duration 通过遍历 MP3 帧头计算音频时长（CBR/VBR 均适用）。
//
// 文件可以带 ID3v2 标签；标签之后的第一个字节必须是帧同步字。解析不到
// 任何合法帧、或连续垃圾数据超过阈值时返回 ErrInvalidMP3。r 必须可 Seek。
// size 为文件总字节数（调用方通过 Stat 获得）。
func MP3Duration(r io.ReadSeeker, size int64) (time.Duration, error) {
	if size <= 0 {
		return 0, ErrInvalidMP3
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek mp3 start: %w", err)
	}
	br := bufio.NewReaderSize(r, mp3ScanBufferSize)

	// 跳过 ID3v2 标签：10 字节头，其中 6-9 字节是 28 位 syncsafe 长度。
	head := make([]byte, 10)
	consumed := int64(0)
	if n, err := io.ReadFull(br, head); err == nil && n == len(head) && string(head[:3]) == "ID3" {
		tagSize := int64(head[6]&0x7f)<<21 | int64(head[7]&0x7f)<<14 |
			int64(head[8]&0x7f)<<7 | int64(head[9]&0x7f)
		skip := tagSize
		if head[5]&0x10 != 0 { // footer present
			skip += 10
		}
		if skip < 0 || 10+skip > size {
			return 0, ErrInvalidMP3
		}
		if _, err := br.Discard(int(skip)); err != nil {
			return 0, ErrInvalidMP3
		}
		consumed = 10 + skip
	} else {
		// 没有 ID3 标签：回到文件起始位置重新扫描。
		if _, err := r.Seek(0, io.SeekStart); err != nil {
			return 0, fmt.Errorf("rewind mp3: %w", err)
		}
		br.Reset(r)
	}

	if first, err := br.Peek(1); err != nil || first[0] != 0xff {
		return 0, ErrInvalidMP3
	}

	frames := 0
	seconds := 0.0
	invalidRun := 0
	for consumed+4 <= size {
		header, err := br.Peek(4)
		if err != nil {
			break
		}
		frame, ok := parseMP3FrameHeader(header)
		if ok && consumed+int64(frame.length) <= size {
			if _, err := br.Discard(frame.length); err != nil {
				break
			}
			consumed += int64(frame.length)
			frames++
			seconds += float64(frame.samples) / float64(frame.sampleRate)
			invalidRun = 0
			continue
		}

		// 非帧头（帧间垃圾、ID3v1 尾部等）：逐字节前移重新同步。
		if _, err := br.Discard(1); err != nil {
			break
		}
		consumed++
		invalidRun++
		if invalidRun > mp3MaxInvalidRun {
			return 0, ErrInvalidMP3
		}
	}
	if frames == 0 {
		return 0, ErrInvalidMP3
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// parseMP3FrameHeader 解析 4 字节帧头。仅接受 MPEG 1/2/2.5 Layer III
// （即 .mp3）；free bitrate（索引 0）与非法索引均视为无效帧。
func parseMP3FrameHeader(h []byte) (mp3Frame, bool) {
	if len(h) < 4 || h[0] != 0xff || h[1]&0xe0 != 0xe0 {
		return mp3Frame{}, false
	}

	versionID := (h[1] >> 3) & 0x03 // 0=MPEG2.5, 2=MPEG2, 3=MPEG1, 1=reserved
	layerID := (h[1] >> 1) & 0x03   // 1=Layer III
	if versionID == 1 || layerID != 1 {
		return mp3Frame{}, false
	}

	bitrateIdx := int(h[2] >> 4)
	sampleRateIdx := int((h[2] >> 2) & 0x03)
	padding := int((h[2] >> 1) & 0x01)
	if bitrateIdx == 0 || bitrateIdx == 15 || sampleRateIdx == 3 {
		return mp3Frame{}, false
	}

	bitrate := mp3BitrateKbps(versionID, bitrateIdx)
	sampleRate := mp3SampleRate(versionID, sampleRateIdx)
	if bitrate == 0 || sampleRate == 0 {
		return mp3Frame{}, false
	}

	// MPEG1 Layer III：144 * bitrate / sampleRate；MPEG2/2.5：72 * bitrate / sampleRate。
	samples := 1152
	if versionID != 3 {
		samples = 576
	}
	length := (samples/8)*bitrate*1000/sampleRate + padding
	if length < 4 {
		return mp3Frame{}, false
	}
	return mp3Frame{length: length, samples: samples, sampleRate: sampleRate}, true
}

// mp3BitrateKbps 返回 Layer III 的比特率表（kbps）。
func mp3BitrateKbps(versionID byte, idx int) int {
	mpeg1 := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	mpeg2 := [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	if versionID == 3 {
		return mpeg1[idx]
	}
	return mpeg2[idx]
}

// mp3SampleRate 返回采样率表（Hz）。
func mp3SampleRate(versionID byte, idx int) int {
	mpeg1 := [...]int{44100, 48000, 32000}
	mpeg2 := [...]int{22050, 24000, 16000}
	mpeg25 := [...]int{11025, 12000, 8000}
	switch versionID {
	case 3:
		return mpeg1[idx]
	case 2:
		return mpeg2[idx]
	default:
		return mpeg25[idx]
	}
}
