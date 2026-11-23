package realtime

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
)

// Minimal RFC 6455 framing. Only what the hub needs: text frames, control
// frames, and fragmented inbound messages. No extensions, no compression.

const (
	opContinuation byte = 0x0
	opText         byte = 0x1
	opBinary       byte = 0x2
	opClose        byte = 0x8
	opPing         byte = 0x9
	opPong         byte = 0xA

	wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

	// Close codes (RFC 6455 section 7.4.1).
	closeNormal      = 1000
	closeGoingAway   = 1001
	closeProtocol    = 1002
	closeUnsupported = 1003
	closePolicy      = 1008
	closeTooBig      = 1009
)

var (
	errProtocol = errors.New("websocket protocol error")
	errTooBig   = errors.New("websocket frame too large")
)

func acceptKey(clientKey string) string {
	sum := sha1.Sum([]byte(clientKey + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// readFrame reads one client frame. Client frames must be masked (RFC 6455
// section 5.1); payloads above maxPayload are rejected before allocation.
func readFrame(r *bufio.Reader, maxPayload int64) (fin bool, opcode byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	if hdr[0]&0x70 != 0 { // RSV bits: no extensions negotiated
		err = errProtocol
		return
	}
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	if !masked {
		err = errProtocol
		return
	}

	length := int64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		length = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		u := binary.BigEndian.Uint64(b[:])
		if u>>63 != 0 {
			err = errProtocol
			return
		}
		length = int64(u)
	}

	isControl := opcode&0x8 != 0
	if isControl && (!fin || length > 125) {
		err = errProtocol
		return
	}
	if length > maxPayload {
		err = errTooBig
		return
	}

	var key [4]byte
	if _, err = io.ReadFull(r, key[:]); err != nil {
		return
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= key[i&3]
	}
	return
}

// encodeFrame builds an unmasked server frame with FIN set.
func encodeFrame(opcode byte, payload []byte) []byte {
	n := len(payload)
	buf := make([]byte, 0, n+10)
	buf = append(buf, 0x80|opcode)
	switch {
	case n < 126:
		buf = append(buf, byte(n))
	case n <= 0xFFFF:
		buf = append(buf, 126, byte(n>>8), byte(n))
	default:
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		buf = append(buf, 127)
		buf = append(buf, l[:]...)
	}
	return append(buf, payload...)
}

func closePayload(code int, reason string) []byte {
	if len(reason) > 100 {
		reason = reason[:100]
	}
	p := make([]byte, 2, 2+len(reason))
	binary.BigEndian.PutUint16(p, uint16(code))
	return append(p, reason...)
}
