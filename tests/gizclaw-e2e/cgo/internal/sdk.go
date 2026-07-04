//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizwebrtc/include -I../../../../c/gizwebrtc/generated
#include "gzc_common.h"
#include "sdk_driver.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
)

func CSDKPing(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "ping", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_ping(identityDir, errbuf, errbufLen)
	})
}

func CSDKServerRuntime(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "server runtime", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_server_runtime(identityDir, errbuf, errbufLen)
	})
}

func CSDKServerStatus(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "server status", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_server_status(identityDir, errbuf, errbufLen)
	})
}

func CSDKSpeedTest(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "speed test", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_speed_test(identityDir, errbuf, errbufLen)
	})
}

func CSDKFirmwareJSON(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "firmware json", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_firmware_json(identityDir, errbuf, errbufLen)
	})
}

func CSDKFirmwareDownload(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "firmware download", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_firmware_download(identityDir, errbuf, errbufLen)
	})
}

func CSDKChatWorkspace(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "chat workspace", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_chat_workspace(identityDir, errbuf, errbufLen)
	})
}

func CSDKChatRoundtrip(t *testing.T, identityDir, workspaceName, oggPath string) {
	t.Helper()
	packetBlob := opusPacketBlobFromOgg(t, oggPath)
	cIdentityDir := C.CString(identityDir)
	defer C.free(unsafe.Pointer(cIdentityDir))
	cWorkspaceName := C.CString(workspaceName)
	defer C.free(unsafe.Pointer(cWorkspaceName))
	cPacketBlob := C.CBytes(packetBlob)
	defer C.free(cPacketBlob)
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_run_chat_roundtrip(
		cIdentityDir,
		cWorkspaceName,
		(*C.uchar)(cPacketBlob),
		C.ulong(len(packetBlob)),
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		t.Fatalf("C SDK chat roundtrip failed rc=%d: %s", int(rc), cString(errbuf))
	}
}

func CSDKSocialBasic(t *testing.T, identityDir string) {
	t.Helper()
	runCSDKScenario(t, identityDir, "social basic", func(identityDir, errbuf *C.char, errbufLen C.ulong) C.int {
		return C.gzc_cgo_run_social_basic(identityDir, errbuf, errbufLen)
	})
}

func CSDKSocialRelationships(t *testing.T, identityADir, identityBDir string) {
	t.Helper()
	cIdentityADir := C.CString(identityADir)
	defer C.free(unsafe.Pointer(cIdentityADir))
	cIdentityBDir := C.CString(identityBDir)
	defer C.free(unsafe.Pointer(cIdentityBDir))
	errbuf := make([]byte, 1024)
	rc := C.gzc_cgo_run_social_relationships(
		cIdentityADir,
		cIdentityBDir,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		t.Fatalf("C SDK social relationships failed rc=%d: %s", int(rc), cString(errbuf))
	}
}

func runCSDKScenario(
	t *testing.T,
	identityDir string,
	name string,
	run func(*C.char, *C.char, C.ulong) C.int,
) {
	t.Helper()
	cIdentityDir := C.CString(identityDir)
	defer C.free(unsafe.Pointer(cIdentityDir))
	errbuf := make([]byte, 1024)
	rc := run(cIdentityDir, (*C.char)(unsafe.Pointer(&errbuf[0])), C.ulong(len(errbuf)))
	if rc != C.GZC_OK {
		t.Fatalf("C SDK %s failed rc=%d: %s", name, int(rc), cString(errbuf))
	}
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return fmt.Sprintf("%q", string(buf))
}

func opusPacketBlobFromOgg(t *testing.T, path string) []byte {
	t.Helper()
	audio, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read opus fixture: %v", err)
	}
	var packets [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			t.Fatalf("read ogg opus packets: %v", err)
		}
		if len(packet.Data) == 0 || bytes.HasPrefix(packet.Data, []byte("OpusHead")) || bytes.HasPrefix(packet.Data, []byte("OpusTags")) {
			continue
		}
		packets = append(packets, append([]byte(nil), packet.Data...))
	}
	if len(packets) == 0 {
		t.Fatal("opus fixture has no opus payload packets")
	}
	var out bytes.Buffer
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(packets)))
	out.Write(header[:])
	for _, packet := range packets {
		binary.LittleEndian.PutUint32(header[:], uint32(len(packet)))
		out.Write(header[:])
		out.Write(packet)
	}
	return out.Bytes()
}
