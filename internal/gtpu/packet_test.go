package gtpu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestParseGPDU(t *testing.T) {
	// flags 0x30 = version 1, PT=1 | type 0xFF | length 0x0001 | teid 0xDEADBEEF
	msg := []byte{
		0x30, 0xFF, 0x00, 0x01,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00, 0x00, 0x00, 0x00,
	}

	header, payload, err := Parse(msg)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if header.MessageType != MsgTypeGPDU {
		t.Errorf("MessageType = %#x, want %#x", header.MessageType, MsgTypeGPDU)
	}
	if header.TEID != 0xDEADBEEF {
		t.Errorf("TEID = %#x, want %#x", header.TEID, uint32(0xDEADBEEF))
	}
	if header.Length != 1 {
		t.Errorf("Length = %d, want 1", header.Length)
	}
	if header.HasSequence {
		t.Error("HasSequence = true, want false")
	}
	if len(payload) != 4 {
		t.Errorf("len(payload) = %d, want 4", len(payload))
	}
}

func TestParseWithSequenceNumber(t *testing.T) {
	// flags 0x34 = version 1, PT=1, S=1
	msg := []byte{
		0x34, 0xFF, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x07,
		0x00, 0x00, 0x00, 0x2A, // sequence number
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	header, payload, err := Parse(msg)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if !header.HasSequence {
		t.Error("HasSequence = false, want true")
	}
	if header.SequenceNumber != 42 {
		t.Errorf("SequenceNumber = %d, want 42", header.SequenceNumber)
	}
	if len(payload) != 8 {
		t.Errorf("len(payload) = %d, want 8", len(payload))
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		msg     []byte
		wantErr error
	}{
		{
			name:    "empty message",
			msg:     []byte{},
			wantErr: ErrShortMessage,
		},
		{
			name:    "truncated header",
			msg:     []byte{0x30, 0xFF, 0x00},
			wantErr: ErrShortMessage,
		},
		{
			name:    "bad version",
			msg:     []byte{0x10, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: ErrBadVersion,
		},
		{
			name:    "not gtp",
			msg:     []byte{0x20, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			wantErr: ErrBadProtocol,
		},
		{
			name:    "length field larger than the message",
			msg:     []byte{0x30, 0xFF, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00},
			wantErr: ErrLengthField,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Parse(tc.msg)
			if err == nil {
				t.Fatalf("Parse() = nil, want %v", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Parse() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	h := &Header{MessageType: MsgTypeGPDU, TEID: 0x01020304}

	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	msg, err := h.Encode(payload)
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	if len(msg) != HeaderLen+len(payload) {
		t.Fatalf("len(msg) = %d, want %d", len(msg), HeaderLen+len(payload))
	}
	if got := binary.BigEndian.Uint16(msg[2:4]); got != 1 {
		t.Errorf("length field = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(msg[4:8]); got != 0x01020304 {
		t.Errorf("TEID = %#x, want %#x", got, uint32(0x01020304))
	}
}

func TestEncodeRejectsUnalignedPayload(t *testing.T) {
	h := &Header{MessageType: MsgTypeGPDU}
	if _, err := h.Encode([]byte{0x01, 0x02, 0x03}); err == nil {
		t.Fatal("Encode() = nil, want an error")
	}
}

func TestEncodeLengthFieldCountsOptionalFields(t *testing.T) {
	// The length field counts everything after the mandatory header, so the
	// 4 byte sequence number must be included in it.
	h := &Header{MessageType: MsgTypeGPDU, TEID: 1, HasSequence: true}

	msg, err := h.Encode(make([]byte, 64))
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	if got := binary.BigEndian.Uint16(msg[2:4]); got != 17 {
		t.Errorf("length field = %d, want 17 (64 payload + 4 sequence)", got)
	}

	header, payload, err := Parse(msg)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(payload) != 64 {
		t.Errorf("len(payload) = %d, want 64", len(payload))
	}
	if !header.HasSequence {
		t.Error("HasSequence = false, want true")
	}
}

func TestEncodeRejectsUnalignedTotalWithPayloadNumber(t *testing.T) {
	// A 2 byte N-PDU number plus a 4 byte payload does not divide into a
	// multiple of 4, which the length field cannot express.
	h := &Header{MessageType: MsgTypeGPDU, HasPayloadNumber: true}
	if _, err := h.Encode(make([]byte, 4)); err == nil {
		t.Fatal("Encode() = nil, want an error")
	}
}

func TestParseWithPayloadNumber(t *testing.T) {
	// flags 0x32 = version 1, PT=1, PN=1
	//
	// The 2 byte N-PDU number is followed by 2 padding bytes so that the
	// length field stays a multiple of 4. The padding belongs to the payload
	// region, so Parse returns 6 bytes: 2 padding + 4 payload.
	msg := []byte{
		0x32, 0xFF, 0x00, 0x02, // length = 2 -> 8 octets follow the header
		0x00, 0x00, 0x00, 0x07, // TEID
		0x00, 0x00, // N-PDU number
		0x00, 0x00, // padding to a multiple of 4
		0xAA, 0xBB, 0xCC, 0xDD,
	}

	header, payload, err := Parse(msg)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if !header.HasPayloadNumber {
		t.Error("HasPayloadNumber = false, want true")
	}
	if len(payload) != 6 {
		t.Errorf("len(payload) = %d, want 6", len(payload))
	}
}

func TestRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte{0x42}, 64)

	h := &Header{
		MessageType:    MsgTypeGPDU,
		TEID:           0xCAFEBABE,
		HasSequence:    true,
		SequenceNumber: 1234,
	}

	msg, err := h.Encode(payload)
	if err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}

	got, gotPayload, err := Parse(msg)
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if got.MessageType != h.MessageType {
		t.Errorf("MessageType = %#x, want %#x", got.MessageType, h.MessageType)
	}
	if got.TEID != h.TEID {
		t.Errorf("TEID = %#x, want %#x", got.TEID, h.TEID)
	}
	if got.SequenceNumber != h.SequenceNumber {
		t.Errorf("SequenceNumber = %d, want %d", got.SequenceNumber, h.SequenceNumber)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload = %x, want %x", gotPayload, payload)
	}
}
