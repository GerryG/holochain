// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------

// implements chain header structures & coding

package holochain

import (
	"bytes"
	"encoding/binary"
	ic "github.com/libp2p/go-libp2p-crypto"
	"io"
	"time"
)

type Signature struct {
	S []byte
}

// Header holds chain links, type, timestamp and signature
type Header struct {
	Type       string
	Time       time.Time
	HeaderLink Hash // link to previous header
	EntryLink  Hash // link to entry
	TypeLink   Hash // link to header of previous header of this type
	Sig        Signature
	//	Meta       interface{}
}

var DEBUG bool

// NewHeader makes Header object linked to a previous Header by hash
func (holo *Holochain) NewHeader(now time.Time, htype string, entry Entry, key ic.PrivKey,
	prev Hash, prevType Hash) (hash Hash, hd_result *Header, err error) {
	var header Header
	header.Type = htype
	header.Time = now
	header.HeaderLink = prev
	header.TypeLink = prevType

	header.EntryLink, err = entry.Sum(holo)
	if err != nil {
		return
	}

	// sign the hash of the entry
	sig, err := key.Sign(header.EntryLink.H)
	if err != nil {
		return
	}
	header.Sig = Signature{S: sig}

	hd_result = &header
	hash, _, err = hd_result.Sum(holo)
	if err != nil {
		return
	}

	hd_result = &header
	return
}

// Sum encodes and creates a hash digest of the header
func (hd *Header) Sum(holo *Holochain) (hash Hash, b []byte, err error) {
	b, err = hd.Marshal(holo)
	if err == nil {
		err = hash.Sum(holo, b)
	}
	return
}

// Marshal writes a header to bytes
func (hd *Header) Marshal(holo *Holochain) (b []byte, err error) {
	var s bytes.Buffer
	if hd == nil {
		Debugf("Marshal, hd nil\n")
	}
	err = MarshalHeader(&s, hd)
	if err == nil {
		b = s.Bytes()
	}
	return
}

// MarshalHeader writes a header to a binary stream
func MarshalHeader(writer io.Writer, hd *Header) (err error) {
	var b []byte
	b = []byte(hd.Type)
	l := uint8(len(b))
	err = binary.Write(writer, binary.LittleEndian, l)
	if err != nil {
		return
	}
	err = binary.Write(writer, binary.LittleEndian, b)
	if err != nil {
		return
	}
	b, err = hd.Time.MarshalBinary()
	err = binary.Write(writer, binary.LittleEndian, b)
	if err != nil {
		return
	}

	err = hd.HeaderLink.MarshalHash(writer)
	if err != nil {
		return
	}

	err = hd.EntryLink.MarshalHash(writer)
	if err != nil {
		return
	}

	err = hd.TypeLink.MarshalHash(writer)
	if err != nil {
		return
	}
	err = MarshalSignature(writer, &hd.Sig)
	if err != nil {
		return
	}

	// write out 0 for future expansion (meta)
	z := uint64(0)
	err = binary.Write(writer, binary.LittleEndian, &z)
	if err != nil {
		return
	}
	return
}

// Unmarshal reads a header from bytes
func (hd *Header) Unmarshal(holo *Holochain, b []byte, hashSize int) (err error) {
	s := bytes.NewBuffer(b)
	err = holo.UnmarshalHeader(s, hd, hashSize)
	return
}

// UnmarshalHeader reads a Header from a binary stream
func (holo *Holochain) UnmarshalHeader(reader io.Reader, hd *Header, hashSize int) (err error) {
	var l uint8
	err = binary.Read(reader, binary.LittleEndian, &l)
	if err != nil {
		return
	}

	var b = make([]byte, l)
	err = binary.Read(reader, binary.LittleEndian, b)
	if err != nil {
		return
	}

	hd.Type = string(b)
	b = make([]byte, 15)
	err = binary.Read(reader, binary.LittleEndian, b)
	if err != nil {
		return
	}
	hd.Time.UnmarshalBinary(b)

	err = hd.HeaderLink.UnmarshalHash(reader)
	if err != nil {
		return
	}

	err = hd.EntryLink.UnmarshalHash(reader)
	if err != nil {
		return
	}

	err = hd.TypeLink.UnmarshalHash(reader)
	if err != nil {
		return
	}

	err = UnmarshalSignature(reader, &hd.Sig)
	if err != nil {
		return
	}

	z := uint64(0)
	err = binary.Read(reader, binary.LittleEndian, &z)
	if err != nil {
		return
	}
	return
}

// MarshalSignature writes a signature to a binary stream
func MarshalSignature(writer io.Writer, s *Signature) (err error) {
	l := uint8(len(s.S))
	err = binary.Write(writer, binary.LittleEndian, l)
	if err != nil {
		return
	}
	err = binary.Write(writer, binary.LittleEndian, s.S)
	if err != nil {
		return
	}
	return
}

// UnmarshalSignature reads a Signature from a binary stream
func UnmarshalSignature(reader io.Reader, s *Signature) (err error) {
	var l uint8
	err = binary.Read(reader, binary.LittleEndian, &l)
	if err != nil {
		return
	}
	var b = make([]byte, l)
	err = binary.Read(reader, binary.LittleEndian, b)
	if err != nil {
		return
	}
	s.S = b
	return
}
