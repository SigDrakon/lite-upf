// Package gtpu implements GTP-U (3GPP TS 29.281) header handling.
//
// The parser is deliberately kept separate from the socket so that the
// header layout can be table-tested without a network.
package gtpu

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// HeaderLen is the size of the mandatory GTP-U v1 header.
const HeaderLen = 8

// Protocol constants from 3GPP TS 29.281.
const (
	// Version is the GTP version carried in the flags octet.
	Version uint8 = 1
	// ProtocolTypeGTP is the PT field value marking a GTP payload.
	// GTP' uses the value 0.
	ProtocolTypeGTP uint8 = 1
)

// Message types used by the user plane.
const (
	MsgTypeEchoRequest     uint8 = 0x01
	MsgTypeEchoResponse    uint8 = 0x02
	MsgTypeErrorIndication uint8 = 0x32
	MsgTypeGPDU            uint8 = 0xFF
)

// Flag bit positions inside the flags octet (bit 8 is the MSB).
const (
	flagVersionMask   uint8 = 0xE0
	flagProtocolType  uint8 = 0x10
	flagExtension     uint8 = 0x08
	flagSequence      uint8 = 0x04
	flagPayloadNumber uint8 = 0x02
)

// Errors returned by the parser.
var (
	ErrShortMessage = errors.New("gtpu: message shorter than the mandatory header")
	ErrBadVersion   = errors.New("gtpu: unsupported protocol version")
	ErrBadProtocol  = errors.New("gtpu: not a GTP payload")
	ErrLengthField  = errors.New("gtpu: length field does not match the message size")
	ErrTruncated    = errors.New("gtpu: message truncated")
)

// Header is a decoded GTP-U header.
type Header struct {
	// MessageType identifies the GTP-U message.
	MessageType uint8
	// TEID identifies the tunnel endpoint.
	TEID uint32
	// Length is the payload length in multiples of 4 octets, excluding the
	// first 8 octets of the message.
	Length uint16
	// SequenceNumber is only meaningful when HasSequence is true.
	SequenceNumber uint32
	// HasSequence reports whether the sequence number field is present.
	HasSequence bool
	// HasExtension reports whether an extension header is present.
	HasExtension bool
	// HasPayloadNumber reports whether the N-PDU number field is present.
	HasPayloadNumber bool
}

// Parse decodes a GTP-U message and returns its header and payload.
//
// The returned payload is a sub-slice of b and is not copied. When the
// N-PDU number flag is set, the padding that keeps the message length a
// multiple of 4 is part of the returned payload.
func Parse(b []byte) (*Header, []byte, error) {
	if len(b) < HeaderLen {
		return nil, nil, fmt.Errorf("%w: got %d bytes, need at least %d",
			ErrShortMessage, len(b), HeaderLen)
	}

	flags := b[0]
	if flags&flagVersionMask>>5 != Version {
		return nil, nil, fmt.Errorf("%w: %d", ErrBadVersion, flags&flagVersionMask>>5)
	}
	if flags&flagProtocolType>>4 != ProtocolTypeGTP {
		return nil, nil, ErrBadProtocol
	}

	h := &Header{
		MessageType:      b[1],
		Length:           binary.BigEndian.Uint16(b[2:4]),
		TEID:             binary.BigEndian.Uint32(b[4:8]),
		HasExtension:     flags&flagExtension != 0,
		HasSequence:      flags&flagSequence != 0,
		HasPayloadNumber: flags&flagPayloadNumber != 0,
	}

	offset := HeaderLen
	if h.HasExtension {
		if len(b) < offset+4 {
			return nil, nil, fmt.Errorf("%w: extension header", ErrTruncated)
		}
		offset += 4
	}
	if h.HasSequence {
		if len(b) < offset+4 {
			return nil, nil, fmt.Errorf("%w: sequence number", ErrTruncated)
		}
		h.SequenceNumber = binary.BigEndian.Uint32(b[offset : offset+4])
		offset += 4
	}
	if h.HasPayloadNumber {
		if len(b) < offset+2 {
			return nil, nil, fmt.Errorf("%w: payload number", ErrTruncated)
		}
		offset += 2
	}

	// The length field counts the octets that follow the first 8, rounded
	// down to a multiple of 4. Those octets include the optional fields, so
	// the payload starts at `offset` and ends at 8 + Length*4.
	expected := int(h.Length) * 4
	if len(b)-HeaderLen < expected {
		return nil, nil, fmt.Errorf("%w: length field claims %d payload bytes, got %d",
			ErrLengthField, expected, len(b)-HeaderLen)
	}
	if len(b) != HeaderLen+expected {
		return nil, nil, fmt.Errorf("%w: length field claims %d payload bytes, got %d",
			ErrLengthField, expected, len(b)-HeaderLen)
	}

	return h, b[offset : HeaderLen+expected], nil
}

// optionalLen returns the size of the optional fields that follow the
// mandatory header.
func (h *Header) optionalLen() int {
	size := 0
	if h.HasExtension {
		size += 4
	}
	if h.HasSequence {
		size += 4
	}
	if h.HasPayloadNumber {
		size += 2
	}
	return size
}

// Marshal encodes the header into its wire representation.
//
// The returned slice contains only the mandatory header plus the optional
// fields that are present.
func (h *Header) Marshal() ([]byte, error) {
	var flags uint8
	flags |= (Version << 5) & flagVersionMask
	flags |= flagProtocolType
	if h.HasExtension {
		flags |= flagExtension
	}
	if h.HasSequence {
		flags |= flagSequence
	}
	if h.HasPayloadNumber {
		flags |= flagPayloadNumber
	}

	size := HeaderLen + h.optionalLen()

	b := make([]byte, size)
	b[0] = flags
	b[1] = h.MessageType
	binary.BigEndian.PutUint16(b[2:4], h.Length)
	binary.BigEndian.PutUint32(b[4:8], h.TEID)

	// The sequence number sits after the optional extension header. The
	// extension header and the N-PDU number are left as zero bytes.
	seqOffset := HeaderLen
	if h.HasExtension {
		seqOffset += 4
	}
	if h.HasSequence {
		binary.BigEndian.PutUint32(b[seqOffset:seqOffset+4], h.SequenceNumber)
	}
	return b, nil
}

// Encode serialises the header followed by payload into a GTP-U message.
//
// The length field is recomputed: it counts every octet that follows the
// mandatory header, including the optional fields, in multiples of 4.
func (h *Header) Encode(payload []byte) ([]byte, error) {
	total := len(payload) + h.optionalLen()
	if total%4 != 0 {
		return nil, fmt.Errorf("gtpu: %d payload bytes plus %d optional bytes is not a multiple of 4",
			len(payload), h.optionalLen())
	}

	h.Length = uint16(total / 4)

	header, err := h.Marshal()
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, len(header)+len(payload))
	out = append(out, header...)
	out = append(out, payload...)
	return out, nil
}
