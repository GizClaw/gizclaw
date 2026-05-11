package stampedopus

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPackUnpackRoundTrip(t *testing.T) {
	const timestamp = uint64(1_741_234_567_890)
	frame := []byte{0xF8, 0xFF, 0x10, 0x20}

	packed := Pack(timestamp, frame)
	if got, want := len(packed), HeaderSize+len(frame); got != want {
		t.Fatalf("packed len=%d, want %d", got, want)
	}
	if packed[0] != Version {
		t.Fatalf("version=%d, want %d", packed[0], Version)
	}

	gotTS, gotFrame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack returned ok=false")
	}
	if gotTS != timestamp {
		t.Fatalf("timestamp=%d, want %d", gotTS, timestamp)
	}
	if !bytes.Equal(gotFrame, frame) {
		t.Fatalf("frame mismatch: got %v, want %v", gotFrame, frame)
	}
}

func TestPackTimestampLow56Bits(t *testing.T) {
	const timestamp = uint64(0xAB0123456789ABCD)
	const payloadByte = 0x42

	packed := Pack(timestamp, []byte{payloadByte})

	wantTS := timestamp & timestampMask
	var wantTSBuf [8]byte
	binary.BigEndian.PutUint64(wantTSBuf[:], wantTS)
	if !bytes.Equal(packed[1:HeaderSize], wantTSBuf[1:]) {
		t.Fatalf("timestamp bytes mismatch: got %x, want %x", packed[1:HeaderSize], wantTSBuf[1:])
	}

	gotTS, gotFrame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack returned ok=false")
	}
	if gotTS != wantTS {
		t.Fatalf("timestamp=%#x, want %#x", gotTS, wantTS)
	}
	if len(gotFrame) != 1 || gotFrame[0] != payloadByte {
		t.Fatalf("frame=%v, want [%d]", gotFrame, payloadByte)
	}
}

func TestUnpackRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "too_short",
			data: make([]byte, HeaderSize-1),
		},
		{
			name: "bad_version",
			data: func() []byte {
				b := Pack(1, []byte{1})
				b[0] = Version + 1
				return b
			}(),
		},
		{
			name: "empty_payload",
			data: Pack(1, nil),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTS, gotFrame, ok := Unpack(tc.data)
			if ok {
				t.Fatalf("Unpack(%q) ok=true, want false", tc.name)
			}
			if gotTS != 0 {
				t.Fatalf("timestamp=%d, want 0", gotTS)
			}
			if gotFrame != nil {
				t.Fatalf("frame=%v, want nil", gotFrame)
			}
		})
	}
}

func TestUnpackReturnsCopiedFrame(t *testing.T) {
	packed := Pack(99, []byte{1, 2, 3})
	_, frame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack returned ok=false")
	}

	packed[HeaderSize] = 9
	if frame[0] != 1 {
		t.Fatalf("frame alias detected: frame[0]=%d, want 1", frame[0])
	}
}

func TestPackCopiesInputFrame(t *testing.T) {
	src := []byte{7, 8, 9}
	packed := Pack(123, src)

	src[0] = 0
	if packed[HeaderSize] != 7 {
		t.Fatalf("pack alias detected: packed[HeaderSize]=%d, want 7", packed[HeaderSize])
	}
}

// TestUnpackNilData 测试 nil data 解包
func TestUnpackNilData(t *testing.T) {
	_, _, ok := Unpack(nil)
	if ok {
		t.Fatal("Unpack(nil) should return ok=false")
	}
}

// TestUnpackVariousInvalidVersions 测试多种无效版本号
func TestUnpackVariousInvalidVersions(t *testing.T) {
	invalidVersions := []byte{0, 2, 128, 255}
	for _, v := range invalidVersions {
		data := Pack(1, []byte{1})
		data[0] = v
		_, _, ok := Unpack(data)
		if ok {
			t.Fatalf("Unpack with version=%d should return ok=false", v)
		}
	}
}

// TestUnpackVariousShortLengths 测试各种短长度
func TestUnpackVariousShortLengths(t *testing.T) {
	for length := range HeaderSize {
		data := make([]byte, length)
		if length > 0 {
			data[0] = Version
		}
		_, _, ok := Unpack(data)
		if ok {
			t.Fatalf("Unpack with length=%d should return ok=false", length)
		}
	}
}

// TestPackWithZeroTimestamp 测试时间戳为 0
func TestPackWithZeroTimestamp(t *testing.T) {
	frame := []byte{0x01, 0x02, 0x03}
	packed := Pack(0, frame)

	gotTS, gotFrame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack should succeed")
	}
	if gotTS != 0 {
		t.Fatalf("timestamp=%d, want 0", gotTS)
	}
	if !bytes.Equal(gotFrame, frame) {
		t.Fatalf("frame mismatch: got %v, want %v", gotFrame, frame)
	}
}

// TestPackZeroTimestampEmptyFrameContract 验证零时间戳 + 空 frame 的 API 合同。
func TestPackZeroTimestampEmptyFrameContract(t *testing.T) {
	t.Run("nil_frame", func(t *testing.T) {
		packed := Pack(0, nil)
		if len(packed) != HeaderSize {
			t.Fatalf("packed len=%d, want %d", len(packed), HeaderSize)
		}
		if packed[0] != Version {
			t.Fatalf("version=%d, want %d", packed[0], Version)
		}

		gotTS, gotFrame, ok := Unpack(packed)
		if ok {
			t.Fatal("Unpack should fail for empty payload")
		}
		if gotTS != 0 {
			t.Fatalf("timestamp=%d, want 0", gotTS)
		}
		if gotFrame != nil {
			t.Fatalf("frame=%v, want nil", gotFrame)
		}
	})

	t.Run("empty_slice_frame", func(t *testing.T) {
		packed := Pack(0, []byte{})
		if len(packed) != HeaderSize {
			t.Fatalf("packed len=%d, want %d", len(packed), HeaderSize)
		}

		_, _, ok := Unpack(packed)
		if ok {
			t.Fatal("Unpack should fail for empty payload")
		}
	})
}

// TestUnpackMinValidLengthSuccess 验证最小合法长度（9 字节）可成功解包。
func TestUnpackMinValidLengthSuccess(t *testing.T) {
	packed := Pack(0x01020304050607, []byte{0x99})
	if len(packed) != HeaderSize+1 {
		t.Fatalf("packed len=%d, want %d", len(packed), HeaderSize+1)
	}

	gotTS, gotFrame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack should succeed for len(data)==9")
	}
	if gotTS != 0x01020304050607 {
		t.Fatalf("timestamp=%#x, want %#x", gotTS, uint64(0x01020304050607))
	}
	if len(gotFrame) != 1 || gotFrame[0] != 0x99 {
		t.Fatalf("frame=%v, want [0x99]", gotFrame)
	}
}

// TestUnpackAcceptsRandomPayloadWhenHeaderValid 验证 header 合法时随机 payload 可正常解包。
func TestUnpackAcceptsRandomPayloadWhenHeaderValid(t *testing.T) {
	timestamp := uint64(0x00112233445566)
	randomPayload := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xFA, 0xCE}
	data := Pack(timestamp, randomPayload)

	gotTS, gotFrame, ok := Unpack(data)
	if !ok {
		t.Fatal("Unpack should succeed for valid header with arbitrary payload")
	}
	if gotTS != timestamp {
		t.Fatalf("timestamp=%#x, want %#x", gotTS, timestamp)
	}
	if !bytes.Equal(gotFrame, randomPayload) {
		t.Fatalf("frame mismatch: got %v, want %v", gotFrame, randomPayload)
	}
}

// TestPackWithLargeFrame 测试大 frame
func TestPackWithLargeFrame(t *testing.T) {
	timestamp := uint64(1234567890)
	frame := make([]byte, 4096) // 4KB frame
	for i := range frame {
		frame[i] = byte(i % 256)
	}

	packed := Pack(timestamp, frame)
	if len(packed) != HeaderSize+len(frame) {
		t.Fatalf("packed length=%d, want %d", len(packed), HeaderSize+len(frame))
	}

	gotTS, gotFrame, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack should succeed")
	}
	if gotTS != timestamp {
		t.Fatalf("timestamp=%d, want %d", gotTS, timestamp)
	}
	if !bytes.Equal(gotFrame, frame) {
		t.Fatal("frame mismatch")
	}
}

// TestUnpackFrameIndependence 测试解包后的 frame 是独立的
func TestUnpackFrameIndependence(t *testing.T) {
	packed := Pack(100, []byte{1, 2, 3, 4, 5})
	_, frame1, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack should succeed")
	}

	// 修改 frame1
	frame1[0] = 99

	// 再次解包，应该得到原始数据
	_, frame2, ok := Unpack(packed)
	if !ok {
		t.Fatal("Unpack should succeed")
	}

	if frame2[0] != 1 {
		t.Fatalf("frame2[0]=%d, want 1 (original data should be unchanged)", frame2[0])
	}
}

// TestMultipleUnpack 测试多次解包同一个 packed 数据
func TestMultipleUnpack(t *testing.T) {
	timestamp := uint64(9876543210)
	frame := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	packed := Pack(timestamp, frame)

	// 多次解包
	for i := range 10 {
		gotTS, gotFrame, ok := Unpack(packed)
		if !ok {
			t.Fatalf("Unpack iteration %d failed", i)
		}
		if gotTS != timestamp {
			t.Fatalf("iteration %d: timestamp=%d, want %d", i, gotTS, timestamp)
		}
		if !bytes.Equal(gotFrame, frame) {
			t.Fatalf("iteration %d: frame mismatch", i)
		}
	}
}
