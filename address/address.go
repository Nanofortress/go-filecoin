package address

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/filecoin-project/go-filecoin/bls-signatures"

	"gx/ipfs/QmSKyB5faguXT4NqbrXpnRXqaVj5DhSm7x9BtzFydBY1UK/go-leb128"
	"gx/ipfs/QmZp3eKdYQHHAneECmeK6HhiMwTPufmjC8DuuaGKv3unvx/blake2b-simd"
	cbor "gx/ipfs/QmcZLyosDwMKdB6NLRsiss9HXzDPhVhhRtPy67JFKTDQDX/go-ipld-cbor"
	"gx/ipfs/QmdBzoMxsBpojBfN1cv5GnKtB7sfYBMoLH7p9qSyEVYXcu/refmt/obj/atlas"
)

func init() {
	cbor.RegisterCborType(addressAtlasEntry)
}

var addressAtlasEntry = atlas.BuildEntry(Address{}).Transform().
	TransformMarshal(atlas.MakeMarshalTransformFunc(
		func(a Address) ([]byte, error) {
			return a.Bytes(), nil
		})).
	TransformUnmarshal(atlas.MakeUnmarshalTransformFunc(
		func(x []byte) (Address, error) {
			return NewFromBytes(x)
		})).
	Complete()

/*

There are 2 ways a filecoin address can be represented. An address appearing on
chain will always be formatted as raw bytes. An address may also be encoded to
a string, this encoding includes a checksum and network prefix. An address
encoded as a string will never appear on chain, this format is used for sharing
among humans.

Bytes:
|----------|---------|
| protocol | payload |
|----------|---------|
|  1 byte  | n bytes |

String:
|------------|----------|---------|----------|
|  network   | protocol | payload | checksum |
|------------|----------|---------|----------|
| 'f' or 't' |  1 byte  | n bytes | 4 bytes  |

*/

// Address is the go type that represents an address in the filecoin network.
type Address struct{ str string }

// Undef is the type that represents an undefined address.
var Undef = Address{}

// Network represents which network an address belongs to.
type Network = byte

const (
	// Mainnet is the main network.
	Mainnet Network = iota
	// Testnet is the test network.
	Testnet
)

// MainnetPrefix is the main network prefix.
const MainnetPrefix = "f"

// TestnetPrefix is the main network prefix.
const TestnetPrefix = "t"

// Protocol represents which protocol an address uses.
type Protocol = byte

const (
	// ID represents the address ID protocol.
	ID Protocol = iota
	// SECP256K1 represents the address SECP256K1 protocol.
	SECP256K1
	// Actor represents the address Actor protocol.
	Actor
	// BLS represents the address BLS protocol.
	BLS
)

// Protocol returns the protocol used by the address.
func (a Address) Protocol() Protocol {
	return a.str[0]
}

// Payload returns the payload of the address.
func (a Address) Payload() []byte {
	return []byte(a.str[1:])
}

// Bytes returns the address as bytes.
func (a Address) Bytes() []byte {
	return []byte(a.str)
}

// String returns an address encoded as a string.
func (a Address) String() string {
	str, err := encode(Testnet, a)
	if err != nil {
		panic(err)
	}
	return str
}

// Equal returns true if address `b` is equal to address.
func (a Address) Equal(b Address) bool {
	return bytes.Equal(a.Bytes(), b.Bytes())
}

// Empty returns true if the address is empty, false otherwise.
func (a Address) Empty() bool {
	return a == Undef
}

// Unmarshal unmarshals the cbor bytes into the address.
func (a Address) Unmarshal(b []byte) error {
	return cbor.DecodeInto(b, &a)
}

// Marshal marshals the address to cbor.
func (a Address) Marshal() ([]byte, error) {
	return cbor.DumpObject(a)
}

// UnmarshalJSON implements the json unmarshal interface.
func (a *Address) UnmarshalJSON(b []byte) error {
	in := strings.TrimSuffix(strings.TrimPrefix(string(b), `"`), `"`)
	addr, err := decode(in)
	if err != nil {
		return err
	}
	*a = addr
	return nil
}

// MarshalJSON implements the json marshal interface.
func (a Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// Format implements the Formatter interface.
func (a Address) Format(f fmt.State, c rune) {
	switch c {
	case 'v':
		fmt.Fprintf(f, "[%x - %x]", a.Protocol(), a.Payload()) // nolint: errcheck
	case 's':
		fmt.Fprintf(f, "%s", a.String()) // nolint: errcheck
	default:
		fmt.Fprintf(f, "%"+string(c), a.Bytes()) // nolint: errcheck
	}
}

// NewIDAddress returns an address using the ID protocol.
func NewIDAddress(id uint64) (Address, error) {
	return newAddress(ID, leb128.FromUInt64(id))
}

// NewSecp256k1Address returns an address using the SECP256K1 protocol.
func NewSecp256k1Address(pubkey []byte) (Address, error) {
	return newAddress(SECP256K1, addressHash(pubkey))
}

// NewActorAddress returns an address using the Actor protocol.
func NewActorAddress(data []byte) (Address, error) {
	return newAddress(Actor, addressHash(data))
}

// NewBLSAddress returns an address using the BLS protocol.
func NewBLSAddress(pubkey []byte) (Address, error) {
	return newAddress(BLS, pubkey)
}

// NewFromString returns the address represented by the string `addr`.
func NewFromString(addr string) (Address, error) {
	return decode(addr)
}

// NewFromBytes return the address represented by the bytes `addr`.
func NewFromBytes(addr []byte) (Address, error) {
	if len(addr) == 0 {
		return Undef, nil
	}
	if len(addr) == 1 {
		return Undef, ErrInvalidLength
	}
	return newAddress(addr[0], addr[1:])
}

// Checksum returns the checksum of `ingest`.
func Checksum(ingest []byte) []byte {
	return hash(ingest, checksumHashConfig)
}

// ValidateChecksum returns true if the checksum of `ingest` is equal to `expected`>
func ValidateChecksum(ingest, expect []byte) bool {
	digest := Checksum(ingest)
	return bytes.Equal(digest, expect)
}

func addressHash(ingest []byte) []byte {
	return hash(ingest, payloadHashConfig)
}

func newAddress(protocol Protocol, payload []byte) (Address, error) {
	switch protocol {
	case ID:
	case SECP256K1, Actor:
		if len(payload) != PayloadHashLength {
			return Undef, ErrInvalidPayload
		}
	case BLS:
		if len(payload) != bls.PublicKeyBytes {
			return Undef, ErrInvalidPayload
		}
	default:
		return Undef, ErrUnknownProtocol
	}
	explen := 1 + len(payload)
	buf := make([]byte, explen)

	buf[0] = protocol
	if c := copy(buf[1:], payload); c != len(payload) {
		panic("copy data length is inconsistent")
	}

	return Address{string(buf)}, nil
}

func encode(network Network, addr Address) (string, error) {
	// DONOTMERGE: what should encoding a Undef address return, and should it evene be allowed?
	if addr == Undef {
		return EmptyAddressString, nil
	}
	var ntwk string
	switch network {
	case Mainnet:
		ntwk = MainnetPrefix
	case Testnet:
		ntwk = TestnetPrefix
	default:
		return EmptyAddressString, ErrUnknownNetwork
	}

	var strAddr string
	switch addr.Protocol() {
	case SECP256K1, Actor, BLS:
		cksm := Checksum(append([]byte{addr.Protocol()}, addr.Payload()...))
		strAddr = ntwk + fmt.Sprintf("%d", addr.Protocol()) + AddressEncoding.WithPadding(-1).EncodeToString(append(addr.Payload(), cksm[:]...))
	case ID:
		strAddr = ntwk + fmt.Sprintf("%d", addr.Protocol()) + fmt.Sprintf("%d", leb128.ToUInt64(addr.Payload()))
	default:
		return EmptyAddressString, ErrUnknownProtocol
	}
	return strAddr, nil
}

func decode(a string) (Address, error) {
	// DONOTMERGE: should decoding a zero length address error?
	if len(a) == 0 {
		return Undef, nil
	}
	if a == EmptyAddressString {
		return Undef, nil
	}
	if len(a) > MaxAddressStringLength || len(a) < 3 {
		return Undef, ErrInvalidLength
	}

	if string(a[0]) != MainnetPrefix && string(a[0]) != TestnetPrefix {
		return Undef, ErrUnknownNetwork
	}

	var protocol Protocol
	switch a[1] {
	case '0':
		protocol = ID
	case '1':
		protocol = SECP256K1
	case '2':
		protocol = Actor
	case '3':
		protocol = BLS
	default:
		return Undef, ErrUnknownProtocol
	}

	raw := a[2:]
	if protocol == ID {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return Undef, ErrInvalidPayload
		}
		return newAddress(protocol, leb128.FromUInt64(id))
	}

	payloadcksm, err := AddressEncoding.WithPadding(-1).DecodeString(raw)
	if err != nil {
		return Undef, err
	}
	payload := payloadcksm[:len(payloadcksm)-ChecksumHashLength]
	cksm := payloadcksm[len(payloadcksm)-ChecksumHashLength:]

	if protocol == SECP256K1 || protocol == Actor {
		if len(payload) != 20 {
			return Undef, ErrInvalidPayload
		}
	}

	if !ValidateChecksum(append([]byte{protocol}, payload...), cksm) {
		return Undef, ErrInvalidChecksum
	}

	return newAddress(protocol, payload)
}

func hash(ingest []byte, cfg *blake2b.Config) []byte {
	hasher, err := blake2b.New(cfg)
	if err != nil {
		// If this happens sth is very wrong.
		panic(fmt.Sprintf("invalid address hash configuration: %v", err))
	}
	if _, err := hasher.Write(ingest); err != nil {
		// blake2bs Write implementation never returns an error in its current
		// setup. So if this happens sth went very wrong.
		panic(fmt.Sprintf("blake2b is unable to process hashes: %v", err))
	}
	return hasher.Sum(nil)
}
