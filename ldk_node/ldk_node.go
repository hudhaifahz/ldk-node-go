package ldk_node

// #include <ldk_node.h>
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"runtime"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"unsafe"
)

// This is needed, because as of go 1.24
// type RustBuffer C.RustBuffer cannot have methods,
// RustBuffer is treated as non-local type
type GoRustBuffer struct {
	inner C.RustBuffer
}

type RustBufferI interface {
	AsReader() *bytes.Reader
	Free()
	ToGoBytes() []byte
	Data() unsafe.Pointer
	Len() uint64
	Capacity() uint64
}

func RustBufferFromExternal(b RustBufferI) GoRustBuffer {
	return GoRustBuffer{
		inner: C.RustBuffer{
			capacity: C.uint64_t(b.Capacity()),
			len:      C.uint64_t(b.Len()),
			data:     (*C.uchar)(b.Data()),
		},
	}
}

func (cb GoRustBuffer) Capacity() uint64 {
	return uint64(cb.inner.capacity)
}

func (cb GoRustBuffer) Len() uint64 {
	return uint64(cb.inner.len)
}

func (cb GoRustBuffer) Data() unsafe.Pointer {
	return unsafe.Pointer(cb.inner.data)
}

func (cb GoRustBuffer) AsReader() *bytes.Reader {
	b := unsafe.Slice((*byte)(cb.inner.data), C.uint64_t(cb.inner.len))
	return bytes.NewReader(b)
}

func (cb GoRustBuffer) Free() {
	rustCall(func(status *C.RustCallStatus) bool {
		C.ffi_ldk_node_rustbuffer_free(cb.inner, status)
		return false
	})
}

func (cb GoRustBuffer) ToGoBytes() []byte {
	return C.GoBytes(unsafe.Pointer(cb.inner.data), C.int(cb.inner.len))
}

func stringToRustBuffer(str string) C.RustBuffer {
	return bytesToRustBuffer([]byte(str))
}

func bytesToRustBuffer(b []byte) C.RustBuffer {
	if len(b) == 0 {
		return C.RustBuffer{}
	}
	// We can pass the pointer along here, as it is pinned
	// for the duration of this call
	foreign := C.ForeignBytes{
		len:  C.int(len(b)),
		data: (*C.uchar)(unsafe.Pointer(&b[0])),
	}

	return rustCall(func(status *C.RustCallStatus) C.RustBuffer {
		return C.ffi_ldk_node_rustbuffer_from_bytes(foreign, status)
	})
}

type BufLifter[GoType any] interface {
	Lift(value RustBufferI) GoType
}

type BufLowerer[GoType any] interface {
	Lower(value GoType) C.RustBuffer
}

type BufReader[GoType any] interface {
	Read(reader io.Reader) GoType
}

type BufWriter[GoType any] interface {
	Write(writer io.Writer, value GoType)
}

func LowerIntoRustBuffer[GoType any](bufWriter BufWriter[GoType], value GoType) C.RustBuffer {
	// This might be not the most efficient way but it does not require knowing allocation size
	// beforehand
	var buffer bytes.Buffer
	bufWriter.Write(&buffer, value)

	bytes, err := io.ReadAll(&buffer)
	if err != nil {
		panic(fmt.Errorf("reading written data: %w", err))
	}
	return bytesToRustBuffer(bytes)
}

func LiftFromRustBuffer[GoType any](bufReader BufReader[GoType], rbuf RustBufferI) GoType {
	defer rbuf.Free()
	reader := rbuf.AsReader()
	item := bufReader.Read(reader)
	if reader.Len() > 0 {
		// TODO: Remove this
		leftover, _ := io.ReadAll(reader)
		panic(fmt.Errorf("Junk remaining in buffer after lifting: %s", string(leftover)))
	}
	return item
}

func rustCallWithError[E any, U any](converter BufReader[*E], callback func(*C.RustCallStatus) U) (U, *E) {
	var status C.RustCallStatus
	returnValue := callback(&status)
	err := checkCallStatus(converter, status)
	return returnValue, err
}

func checkCallStatus[E any](converter BufReader[*E], status C.RustCallStatus) *E {
	switch status.code {
	case 0:
		return nil
	case 1:
		return LiftFromRustBuffer(converter, GoRustBuffer{inner: status.errorBuf})
	case 2:
		// when the rust code sees a panic, it tries to construct a rustBuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(GoRustBuffer{inner: status.errorBuf})))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		panic(fmt.Errorf("unknown status code: %d", status.code))
	}
}

func checkCallStatusUnknown(status C.RustCallStatus) error {
	switch status.code {
	case 0:
		return nil
	case 1:
		panic(fmt.Errorf("function not returning an error returned an error"))
	case 2:
		// when the rust code sees a panic, it tries to construct a C.RustBuffer
		// with the message.  but if that code panics, then it just sends back
		// an empty buffer.
		if status.errorBuf.len > 0 {
			panic(fmt.Errorf("%s", FfiConverterStringINSTANCE.Lift(GoRustBuffer{
				inner: status.errorBuf,
			})))
		} else {
			panic(fmt.Errorf("Rust panicked while handling Rust panic"))
		}
	default:
		return fmt.Errorf("unknown status code: %d", status.code)
	}
}

func rustCall[U any](callback func(*C.RustCallStatus) U) U {
	returnValue, err := rustCallWithError[error](nil, callback)
	if err != nil {
		panic(err)
	}
	return returnValue
}

type NativeError interface {
	AsError() error
}

func writeInt8(writer io.Writer, value int8) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint8(writer io.Writer, value uint8) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt16(writer io.Writer, value int16) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint16(writer io.Writer, value uint16) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt32(writer io.Writer, value int32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint32(writer io.Writer, value uint32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeInt64(writer io.Writer, value int64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeUint64(writer io.Writer, value uint64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeFloat32(writer io.Writer, value float32) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func writeFloat64(writer io.Writer, value float64) {
	if err := binary.Write(writer, binary.BigEndian, value); err != nil {
		panic(err)
	}
}

func readInt8(reader io.Reader) int8 {
	var result int8
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint8(reader io.Reader) uint8 {
	var result uint8
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt16(reader io.Reader) int16 {
	var result int16
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint16(reader io.Reader) uint16 {
	var result uint16
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt32(reader io.Reader) int32 {
	var result int32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint32(reader io.Reader) uint32 {
	var result uint32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readInt64(reader io.Reader) int64 {
	var result int64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readUint64(reader io.Reader) uint64 {
	var result uint64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readFloat32(reader io.Reader) float32 {
	var result float32
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func readFloat64(reader io.Reader) float64 {
	var result float64
	if err := binary.Read(reader, binary.BigEndian, &result); err != nil {
		panic(err)
	}
	return result
}

func init() {

	FfiConverterLogWriterINSTANCE.register()
	uniffiCheckChecksums()
}

func uniffiCheckChecksums() {
	// Get the bindings contract version from our ComponentInterface
	bindingsContractVersion := 26
	// Get the scaffolding contract version by calling the into the dylib
	scaffoldingContractVersion := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint32_t {
		return C.ffi_ldk_node_uniffi_contract_version()
	})
	if bindingsContractVersion != int(scaffoldingContractVersion) {
		// If this happens try cleaning and rebuilding your project
		panic("ldk_node: UniFFI contract version mismatch")
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_func_default_config()
		})
		if checksum != 55381 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_func_default_config: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_func_generate_entropy_mnemonic()
		})
		if checksum != 48014 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_func_generate_entropy_mnemonic: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_func_lsps2_compute_opening_fee_msat()
		})
		if checksum != 35888 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_func_lsps2_compute_opening_fee_msat: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_amount_milli_satoshis()
		})
		if checksum != 50823 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_amount_milli_satoshis: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_currency()
		})
		if checksum != 32179 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_currency: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_expiry_time_seconds()
		})
		if checksum != 23625 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_expiry_time_seconds: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_fallback_addresses()
		})
		if checksum != 55276 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_fallback_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_invoice_description()
		})
		if checksum != 395 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_invoice_description: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_is_expired()
		})
		if checksum != 15932 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_is_expired: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_min_final_cltv_expiry_delta()
		})
		if checksum != 8855 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_min_final_cltv_expiry_delta: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_network()
		})
		if checksum != 10420 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_network: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_payment_hash()
		})
		if checksum != 42571 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_payment_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_payment_secret()
		})
		if checksum != 26081 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_payment_secret: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_recover_payee_pub_key()
		})
		if checksum != 18874 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_recover_payee_pub_key: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_route_hints()
		})
		if checksum != 63051 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_route_hints: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_seconds_since_epoch()
		})
		if checksum != 53979 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_seconds_since_epoch: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_seconds_until_expiry()
		})
		if checksum != 64193 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_seconds_until_expiry: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_signable_hash()
		})
		if checksum != 30910 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_signable_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11invoice_would_expire()
		})
		if checksum != 30331 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11invoice_would_expire: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_claim_for_hash()
		})
		if checksum != 52848 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_claim_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_fail_for_hash()
		})
		if checksum != 24516 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_fail_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive()
		})
		if checksum != 6073 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_for_hash()
		})
		if checksum != 27050 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_for_hash_with_min_cltv_expiry_delta()
		})
		if checksum != 47514 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_for_hash_with_min_cltv_expiry_delta: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount()
		})
		if checksum != 4893 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_for_hash()
		})
		if checksum != 1402 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_for_hash_with_min_cltv_expiry_delta()
		})
		if checksum != 10331 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_for_hash_with_min_cltv_expiry_delta: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_via_jit_channel()
		})
		if checksum != 24506 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_via_jit_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_via_jit_channel_for_hash()
		})
		if checksum != 38025 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_variable_amount_via_jit_channel_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_via_jit_channel()
		})
		if checksum != 16532 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_via_jit_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_receive_via_jit_channel_for_hash()
		})
		if checksum != 1143 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_receive_via_jit_channel_for_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_send()
		})
		if checksum != 12953 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_send_probes()
		})
		if checksum != 19286 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_send_probes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_send_probes_using_amount()
		})
		if checksum != 5976 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_send_probes_using_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_send_using_amount()
		})
		if checksum != 42793 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_send_using_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt11payment_send_with_first_hop()
		})
		if checksum != 21987 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt11payment_send_with_first_hop: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_absolute_expiry_seconds()
		})
		if checksum != 28589 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_absolute_expiry_seconds: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_amount()
		})
		if checksum != 5213 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_amount_msats()
		})
		if checksum != 9297 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_amount_msats: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_chain()
		})
		if checksum != 3308 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_chain: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_created_at()
		})
		if checksum != 56866 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_created_at: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_encode()
		})
		if checksum != 13200 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_encode: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_fallback_addresses()
		})
		if checksum != 7925 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_fallback_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_invoice_description()
		})
		if checksum != 1713 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_invoice_description: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_is_expired()
		})
		if checksum != 39560 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_is_expired: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_issuer()
		})
		if checksum != 65270 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_issuer: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_issuer_signing_pubkey()
		})
		if checksum != 55411 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_issuer_signing_pubkey: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_metadata()
		})
		if checksum != 37374 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_metadata: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_offer_chains()
		})
		if checksum != 39622 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_offer_chains: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_payer_note()
		})
		if checksum != 28018 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_payer_note: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_payer_signing_pubkey()
		})
		if checksum != 12798 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_payer_signing_pubkey: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_payment_hash()
		})
		if checksum != 63778 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_payment_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_quantity()
		})
		if checksum != 43105 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_quantity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_relative_expiry()
		})
		if checksum != 14024 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_relative_expiry: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_signable_hash()
		})
		if checksum != 39303 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_signable_hash: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12invoice_signing_pubkey()
		})
		if checksum != 35202 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12invoice_signing_pubkey: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_blinded_paths_for_async_recipient()
		})
		if checksum != 14695 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_blinded_paths_for_async_recipient: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_initiate_refund()
		})
		if checksum != 15019 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_initiate_refund: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_receive()
		})
		if checksum != 59252 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_receive: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_receive_async()
		})
		if checksum != 23867 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_receive_async: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_receive_variable_amount()
		})
		if checksum != 35484 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_receive_variable_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_request_refund_payment()
		})
		if checksum != 43248 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_request_refund_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_send()
		})
		if checksum != 27679 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_send_using_amount()
		})
		if checksum != 33255 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_send_using_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_bolt12payment_set_paths_to_static_invoice_server()
		})
		if checksum != 20921 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_bolt12payment_set_paths_to_static_invoice_server: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_build()
		})
		if checksum != 785 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_build: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_build_with_fs_store()
		})
		if checksum != 61304 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_build_with_fs_store: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_build_with_vss_store()
		})
		if checksum != 2871 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_build_with_vss_store: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_build_with_vss_store_and_fixed_headers()
		})
		if checksum != 24910 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_build_with_vss_store_and_fixed_headers: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_migrate_storage()
		})
		if checksum != 3140 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_migrate_storage: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_reset_state()
		})
		if checksum != 46877 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_reset_state: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_restore_encoded_channel_monitors()
		})
		if checksum != 50877 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_restore_encoded_channel_monitors: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_announcement_addresses()
		})
		if checksum != 39271 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_announcement_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_async_payments_role()
		})
		if checksum != 16463 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_async_payments_role: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_chain_source_bitcoind_rest()
		})
		if checksum != 37382 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_chain_source_bitcoind_rest: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_chain_source_bitcoind_rpc()
		})
		if checksum != 2111 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_chain_source_bitcoind_rpc: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_chain_source_electrum()
		})
		if checksum != 55552 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_chain_source_electrum: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_chain_source_esplora()
		})
		if checksum != 1781 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_chain_source_esplora: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_custom_logger()
		})
		if checksum != 51232 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_custom_logger: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_entropy_bip39_mnemonic()
		})
		if checksum != 827 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_entropy_bip39_mnemonic: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_entropy_seed_bytes()
		})
		if checksum != 44799 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_entropy_seed_bytes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_entropy_seed_path()
		})
		if checksum != 64056 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_entropy_seed_path: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_filesystem_logger()
		})
		if checksum != 10249 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_filesystem_logger: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_gossip_source_p2p()
		})
		if checksum != 9279 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_gossip_source_p2p: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_gossip_source_rgs()
		})
		if checksum != 64312 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_gossip_source_rgs: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_liquidity_source_lsps1()
		})
		if checksum != 51527 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_liquidity_source_lsps1: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_liquidity_source_lsps2()
		})
		if checksum != 14430 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_liquidity_source_lsps2: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_listening_addresses()
		})
		if checksum != 14051 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_listening_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_log_facade_logger()
		})
		if checksum != 58410 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_log_facade_logger: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_network()
		})
		if checksum != 27539 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_network: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_node_alias()
		})
		if checksum != 18342 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_node_alias: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_pathfinding_scores_source()
		})
		if checksum != 63501 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_pathfinding_scores_source: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_builder_set_storage_dir_path()
		})
		if checksum != 59019 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_builder_set_storage_dir_path: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_feerate_to_sat_per_kwu()
		})
		if checksum != 58911 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_feerate_to_sat_per_kwu: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_feerate_to_sat_per_vb_ceil()
		})
		if checksum != 58575 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_feerate_to_sat_per_vb_ceil: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_feerate_to_sat_per_vb_floor()
		})
		if checksum != 59617 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_feerate_to_sat_per_vb_floor: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_lsps1liquidity_check_order_status()
		})
		if checksum != 57147 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_lsps1liquidity_check_order_status: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_lsps1liquidity_request_channel()
		})
		if checksum != 18153 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_lsps1liquidity_request_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_lsps2liquidity_request_opening_fee_params()
		})
		if checksum != 44958 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_lsps2liquidity_request_opening_fee_params: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_logwriter_log()
		})
		if checksum != 3299 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_logwriter_log: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_networkgraph_channel()
		})
		if checksum != 38070 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_networkgraph_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_networkgraph_list_channels()
		})
		if checksum != 4693 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_networkgraph_list_channels: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_networkgraph_list_nodes()
		})
		if checksum != 36715 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_networkgraph_list_nodes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_networkgraph_node()
		})
		if checksum != 48925 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_networkgraph_node: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_announcement_addresses()
		})
		if checksum != 61426 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_announcement_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_bolt11_payment()
		})
		if checksum != 41402 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_bolt11_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_bolt12_payment()
		})
		if checksum != 49254 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_bolt12_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_close_channel()
		})
		if checksum != 62479 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_close_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_config()
		})
		if checksum != 7511 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_config: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_connect()
		})
		if checksum != 34120 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_connect: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_disconnect()
		})
		if checksum != 43538 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_disconnect: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_event_handled()
		})
		if checksum != 38712 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_event_handled: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_export_pathfinding_scores()
		})
		if checksum != 62331 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_export_pathfinding_scores: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_force_close_channel()
		})
		if checksum != 48831 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_force_close_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_get_encoded_channel_monitors()
		})
		if checksum != 8363 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_get_encoded_channel_monitors: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_list_balances()
		})
		if checksum != 57528 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_list_balances: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_list_channel_monitor_sizes()
		})
		if checksum != 34459 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_list_channel_monitor_sizes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_list_channels()
		})
		if checksum != 7954 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_list_channels: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_list_payments()
		})
		if checksum != 35002 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_list_payments: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_list_peers()
		})
		if checksum != 14889 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_list_peers: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_listening_addresses()
		})
		if checksum != 2665 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_listening_addresses: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_lsps1_liquidity()
		})
		if checksum != 38201 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_lsps1_liquidity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_lsps2_liquidity()
		})
		if checksum != 59985 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_lsps2_liquidity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_network_graph()
		})
		if checksum != 2695 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_network_graph: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_next_event()
		})
		if checksum != 7682 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_next_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_next_event_async()
		})
		if checksum != 25426 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_next_event_async: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_node_alias()
		})
		if checksum != 29526 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_node_alias: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_node_id()
		})
		if checksum != 51489 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_node_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_onchain_payment()
		})
		if checksum != 6092 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_onchain_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_open_announced_channel()
		})
		if checksum != 36623 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_open_announced_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_open_channel()
		})
		if checksum != 40283 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_open_channel: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_payment()
		})
		if checksum != 60296 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_remove_payment()
		})
		if checksum != 47952 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_remove_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_sign_message()
		})
		if checksum != 49319 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_sign_message: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_splice_in()
		})
		if checksum != 46431 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_splice_in: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_splice_out()
		})
		if checksum != 22115 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_splice_out: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_spontaneous_payment()
		})
		if checksum != 37403 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_spontaneous_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_start()
		})
		if checksum != 58480 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_start: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_status()
		})
		if checksum != 55952 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_status: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_stop()
		})
		if checksum != 42188 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_stop: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_sync_wallets()
		})
		if checksum != 32474 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_sync_wallets: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_unified_qr_payment()
		})
		if checksum != 9837 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_unified_qr_payment: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_update_channel_config()
		})
		if checksum != 37852 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_update_channel_config: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_update_fee_estimates()
		})
		if checksum != 52349 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_update_fee_estimates: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_verify_signature()
		})
		if checksum != 20486 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_verify_signature: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_node_wait_next_event()
		})
		if checksum != 55101 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_node_wait_next_event: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_absolute_expiry_seconds()
		})
		if checksum != 22836 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_absolute_expiry_seconds: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_amount()
		})
		if checksum != 59890 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_amount: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_chains()
		})
		if checksum != 59522 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_chains: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_expects_quantity()
		})
		if checksum != 58457 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_expects_quantity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_id()
		})
		if checksum != 8391 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_id: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_is_expired()
		})
		if checksum != 22651 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_is_expired: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_is_valid_quantity()
		})
		if checksum != 58469 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_is_valid_quantity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_issuer()
		})
		if checksum != 41632 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_issuer: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_issuer_signing_pubkey()
		})
		if checksum != 38162 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_issuer_signing_pubkey: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_metadata()
		})
		if checksum != 18979 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_metadata: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_offer_description()
		})
		if checksum != 11122 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_offer_description: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_offer_supports_chain()
		})
		if checksum != 2135 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_offer_supports_chain: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_onchainpayment_new_address()
		})
		if checksum != 37251 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_onchainpayment_new_address: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_onchainpayment_send_all_to_address()
		})
		if checksum != 37748 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_onchainpayment_send_all_to_address: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_onchainpayment_send_to_address()
		})
		if checksum != 55646 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_onchainpayment_send_to_address: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_absolute_expiry_seconds()
		})
		if checksum != 43722 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_absolute_expiry_seconds: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_amount_msats()
		})
		if checksum != 26467 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_amount_msats: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_chain()
		})
		if checksum != 36565 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_chain: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_is_expired()
		})
		if checksum != 10232 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_is_expired: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_issuer()
		})
		if checksum != 40306 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_issuer: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_payer_metadata()
		})
		if checksum != 23501 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_payer_metadata: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_payer_note()
		})
		if checksum != 47799 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_payer_note: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_payer_signing_pubkey()
		})
		if checksum != 40880 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_payer_signing_pubkey: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_quantity()
		})
		if checksum != 15192 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_quantity: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_refund_refund_description()
		})
		if checksum != 39295 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_refund_refund_description: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_spontaneouspayment_send()
		})
		if checksum != 27905 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_spontaneouspayment_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_spontaneouspayment_send_probes()
		})
		if checksum != 25937 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_spontaneouspayment_send_probes: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_custom_tlvs()
		})
		if checksum != 17876 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_custom_tlvs: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_preimage()
		})
		if checksum != 30854 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_preimage: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_preimage_and_custom_tlvs()
		})
		if checksum != 12104 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_spontaneouspayment_send_with_preimage_and_custom_tlvs: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_unifiedqrpayment_receive()
		})
		if checksum != 913 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_unifiedqrpayment_receive: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_method_unifiedqrpayment_send()
		})
		if checksum != 28285 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_method_unifiedqrpayment_send: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_bolt11invoice_from_str()
		})
		if checksum != 349 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_bolt11invoice_from_str: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_bolt12invoice_from_str()
		})
		if checksum != 22276 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_bolt12invoice_from_str: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_builder_from_config()
		})
		if checksum != 994 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_builder_from_config: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_builder_new()
		})
		if checksum != 40499 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_builder_new: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_feerate_from_sat_per_kwu()
		})
		if checksum != 50548 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_feerate_from_sat_per_kwu: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_feerate_from_sat_per_vb_unchecked()
		})
		if checksum != 41808 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_feerate_from_sat_per_vb_unchecked: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_offer_from_str()
		})
		if checksum != 37070 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_offer_from_str: UniFFI API checksum mismatch")
		}
	}
	{
		checksum := rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint16_t {
			return C.uniffi_ldk_node_checksum_constructor_refund_from_str()
		})
		if checksum != 64884 {
			// If this happens try cleaning and rebuilding your project
			panic("ldk_node: uniffi_ldk_node_checksum_constructor_refund_from_str: UniFFI API checksum mismatch")
		}
	}
}

type FfiConverterUint8 struct{}

var FfiConverterUint8INSTANCE = FfiConverterUint8{}

func (FfiConverterUint8) Lower(value uint8) C.uint8_t {
	return C.uint8_t(value)
}

func (FfiConverterUint8) Write(writer io.Writer, value uint8) {
	writeUint8(writer, value)
}

func (FfiConverterUint8) Lift(value C.uint8_t) uint8 {
	return uint8(value)
}

func (FfiConverterUint8) Read(reader io.Reader) uint8 {
	return readUint8(reader)
}

type FfiDestroyerUint8 struct{}

func (FfiDestroyerUint8) Destroy(_ uint8) {}

type FfiConverterUint16 struct{}

var FfiConverterUint16INSTANCE = FfiConverterUint16{}

func (FfiConverterUint16) Lower(value uint16) C.uint16_t {
	return C.uint16_t(value)
}

func (FfiConverterUint16) Write(writer io.Writer, value uint16) {
	writeUint16(writer, value)
}

func (FfiConverterUint16) Lift(value C.uint16_t) uint16 {
	return uint16(value)
}

func (FfiConverterUint16) Read(reader io.Reader) uint16 {
	return readUint16(reader)
}

type FfiDestroyerUint16 struct{}

func (FfiDestroyerUint16) Destroy(_ uint16) {}

type FfiConverterUint32 struct{}

var FfiConverterUint32INSTANCE = FfiConverterUint32{}

func (FfiConverterUint32) Lower(value uint32) C.uint32_t {
	return C.uint32_t(value)
}

func (FfiConverterUint32) Write(writer io.Writer, value uint32) {
	writeUint32(writer, value)
}

func (FfiConverterUint32) Lift(value C.uint32_t) uint32 {
	return uint32(value)
}

func (FfiConverterUint32) Read(reader io.Reader) uint32 {
	return readUint32(reader)
}

type FfiDestroyerUint32 struct{}

func (FfiDestroyerUint32) Destroy(_ uint32) {}

type FfiConverterUint64 struct{}

var FfiConverterUint64INSTANCE = FfiConverterUint64{}

func (FfiConverterUint64) Lower(value uint64) C.uint64_t {
	return C.uint64_t(value)
}

func (FfiConverterUint64) Write(writer io.Writer, value uint64) {
	writeUint64(writer, value)
}

func (FfiConverterUint64) Lift(value C.uint64_t) uint64 {
	return uint64(value)
}

func (FfiConverterUint64) Read(reader io.Reader) uint64 {
	return readUint64(reader)
}

type FfiDestroyerUint64 struct{}

func (FfiDestroyerUint64) Destroy(_ uint64) {}

type FfiConverterBool struct{}

var FfiConverterBoolINSTANCE = FfiConverterBool{}

func (FfiConverterBool) Lower(value bool) C.int8_t {
	if value {
		return C.int8_t(1)
	}
	return C.int8_t(0)
}

func (FfiConverterBool) Write(writer io.Writer, value bool) {
	if value {
		writeInt8(writer, 1)
	} else {
		writeInt8(writer, 0)
	}
}

func (FfiConverterBool) Lift(value C.int8_t) bool {
	return value != 0
}

func (FfiConverterBool) Read(reader io.Reader) bool {
	return readInt8(reader) != 0
}

type FfiDestroyerBool struct{}

func (FfiDestroyerBool) Destroy(_ bool) {}

type FfiConverterString struct{}

var FfiConverterStringINSTANCE = FfiConverterString{}

func (FfiConverterString) Lift(rb RustBufferI) string {
	defer rb.Free()
	reader := rb.AsReader()
	b, err := io.ReadAll(reader)
	if err != nil {
		panic(fmt.Errorf("reading reader: %w", err))
	}
	return string(b)
}

func (FfiConverterString) Read(reader io.Reader) string {
	length := readInt32(reader)
	buffer := make([]byte, length)
	read_length, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		panic(err)
	}
	if read_length != int(length) {
		panic(fmt.Errorf("bad read length when reading string, expected %d, read %d", length, read_length))
	}
	return string(buffer)
}

func (FfiConverterString) Lower(value string) C.RustBuffer {
	return stringToRustBuffer(value)
}

func (FfiConverterString) Write(writer io.Writer, value string) {
	if len(value) > math.MaxInt32 {
		panic("String is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	write_length, err := io.WriteString(writer, value)
	if err != nil {
		panic(err)
	}
	if write_length != len(value) {
		panic(fmt.Errorf("bad write length when writing string, expected %d, written %d", len(value), write_length))
	}
}

type FfiDestroyerString struct{}

func (FfiDestroyerString) Destroy(_ string) {}

type FfiConverterBytes struct{}

var FfiConverterBytesINSTANCE = FfiConverterBytes{}

func (c FfiConverterBytes) Lower(value []byte) C.RustBuffer {
	return LowerIntoRustBuffer[[]byte](c, value)
}

func (c FfiConverterBytes) Write(writer io.Writer, value []byte) {
	if len(value) > math.MaxInt32 {
		panic("[]byte is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	write_length, err := writer.Write(value)
	if err != nil {
		panic(err)
	}
	if write_length != len(value) {
		panic(fmt.Errorf("bad write length when writing []byte, expected %d, written %d", len(value), write_length))
	}
}

func (c FfiConverterBytes) Lift(rb RustBufferI) []byte {
	return LiftFromRustBuffer[[]byte](c, rb)
}

func (c FfiConverterBytes) Read(reader io.Reader) []byte {
	length := readInt32(reader)
	buffer := make([]byte, length)
	read_length, err := reader.Read(buffer)
	if err != nil && err != io.EOF {
		panic(err)
	}
	if read_length != int(length) {
		panic(fmt.Errorf("bad read length when reading []byte, expected %d, read %d", length, read_length))
	}
	return buffer
}

type FfiDestroyerBytes struct{}

func (FfiDestroyerBytes) Destroy(_ []byte) {}

// Below is an implementation of synchronization requirements outlined in the link.
// https://github.com/mozilla/uniffi-rs/blob/0dc031132d9493ca812c3af6e7dd60ad2ea95bf0/uniffi_bindgen/src/bindings/kotlin/templates/ObjectRuntime.kt#L31

type FfiObject struct {
	pointer       unsafe.Pointer
	callCounter   atomic.Int64
	cloneFunction func(unsafe.Pointer, *C.RustCallStatus) unsafe.Pointer
	freeFunction  func(unsafe.Pointer, *C.RustCallStatus)
	destroyed     atomic.Bool
}

func newFfiObject(
	pointer unsafe.Pointer,
	cloneFunction func(unsafe.Pointer, *C.RustCallStatus) unsafe.Pointer,
	freeFunction func(unsafe.Pointer, *C.RustCallStatus),
) FfiObject {
	return FfiObject{
		pointer:       pointer,
		cloneFunction: cloneFunction,
		freeFunction:  freeFunction,
	}
}

func (ffiObject *FfiObject) incrementPointer(debugName string) unsafe.Pointer {
	for {
		counter := ffiObject.callCounter.Load()
		if counter <= -1 {
			panic(fmt.Errorf("%v object has already been destroyed", debugName))
		}
		if counter == math.MaxInt64 {
			panic(fmt.Errorf("%v object call counter would overflow", debugName))
		}
		if ffiObject.callCounter.CompareAndSwap(counter, counter+1) {
			break
		}
	}

	return rustCall(func(status *C.RustCallStatus) unsafe.Pointer {
		return ffiObject.cloneFunction(ffiObject.pointer, status)
	})
}

func (ffiObject *FfiObject) decrementPointer() {
	if ffiObject.callCounter.Add(-1) == -1 {
		ffiObject.freeRustArcPtr()
	}
}

func (ffiObject *FfiObject) destroy() {
	if ffiObject.destroyed.CompareAndSwap(false, true) {
		if ffiObject.callCounter.Add(-1) == -1 {
			ffiObject.freeRustArcPtr()
		}
	}
}

func (ffiObject *FfiObject) freeRustArcPtr() {
	rustCall(func(status *C.RustCallStatus) int32 {
		ffiObject.freeFunction(ffiObject.pointer, status)
		return 0
	})
}

type Bolt11InvoiceInterface interface {
	AmountMilliSatoshis() *uint64
	Currency() Currency
	ExpiryTimeSeconds() uint64
	FallbackAddresses() []Address
	InvoiceDescription() Bolt11InvoiceDescription
	IsExpired() bool
	MinFinalCltvExpiryDelta() uint64
	Network() Network
	PaymentHash() PaymentHash
	PaymentSecret() PaymentSecret
	RecoverPayeePubKey() PublicKey
	RouteHints() [][]RouteHintHop
	SecondsSinceEpoch() uint64
	SecondsUntilExpiry() uint64
	SignableHash() []uint8
	WouldExpire(atTimeSeconds uint64) bool
}
type Bolt11Invoice struct {
	ffiObject FfiObject
}

func Bolt11InvoiceFromStr(invoiceStr string) (*Bolt11Invoice, error) {
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_bolt11invoice_from_str(FfiConverterStringINSTANCE.Lower(invoiceStr), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Invoice) AmountMilliSatoshis() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_amount_milli_satoshis(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) Currency() Currency {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterCurrencyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_currency(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) ExpiryTimeSeconds() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_expiry_time_seconds(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) FallbackAddresses() []Address {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceTypeAddressINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_fallback_addresses(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) InvoiceDescription() Bolt11InvoiceDescription {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBolt11InvoiceDescriptionINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_invoice_description(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) IsExpired() bool {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_is_expired(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) MinFinalCltvExpiryDelta() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_min_final_cltv_expiry_delta(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) Network() Network {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNetworkINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_network(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) PaymentHash() PaymentHash {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePaymentHashINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_payment_hash(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) PaymentSecret() PaymentSecret {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePaymentSecretINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_payment_secret(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) RecoverPayeePubKey() PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_recover_payee_pub_key(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) RouteHints() [][]RouteHintHop {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceSequenceRouteHintHopINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_route_hints(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) SecondsSinceEpoch() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_seconds_since_epoch(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) SecondsUntilExpiry() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_seconds_until_expiry(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) SignableHash() []uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_signable_hash(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) WouldExpire(atTimeSeconds uint64) bool {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_would_expire(
			_pointer, FfiConverterUint64INSTANCE.Lower(atTimeSeconds), _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) DebugString() string {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_uniffi_trait_debug(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) String() string {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11invoice_uniffi_trait_display(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt11Invoice) Eq(other *Bolt11Invoice) bool {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_uniffi_trait_eq_eq(
			_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (_self *Bolt11Invoice) Ne(other *Bolt11Invoice) bool {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_bolt11invoice_uniffi_trait_eq_ne(
			_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (object *Bolt11Invoice) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterBolt11Invoice struct{}

var FfiConverterBolt11InvoiceINSTANCE = FfiConverterBolt11Invoice{}

func (c FfiConverterBolt11Invoice) Lift(pointer unsafe.Pointer) *Bolt11Invoice {
	result := &Bolt11Invoice{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_bolt11invoice(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_bolt11invoice(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Bolt11Invoice).Destroy)
	return result
}

func (c FfiConverterBolt11Invoice) Read(reader io.Reader) *Bolt11Invoice {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterBolt11Invoice) Lower(value *Bolt11Invoice) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Bolt11Invoice")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterBolt11Invoice) Write(writer io.Writer, value *Bolt11Invoice) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerBolt11Invoice struct{}

func (_ FfiDestroyerBolt11Invoice) Destroy(value *Bolt11Invoice) {
	value.Destroy()
}

type Bolt11PaymentInterface interface {
	ClaimForHash(paymentHash PaymentHash, claimableAmountMsat uint64, preimage PaymentPreimage) error
	FailForHash(paymentHash PaymentHash) error
	Receive(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32) (*Bolt11Invoice, error)
	ReceiveForHash(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash) (*Bolt11Invoice, error)
	ReceiveForHashWithMinCltvExpiryDelta(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash, minCltvExpiryDelta uint16) (*Bolt11Invoice, error)
	ReceiveVariableAmount(description Bolt11InvoiceDescription, expirySecs uint32) (*Bolt11Invoice, error)
	ReceiveVariableAmountForHash(description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash) (*Bolt11Invoice, error)
	ReceiveVariableAmountForHashWithMinCltvExpiryDelta(description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash, minCltvExpiryDelta uint16) (*Bolt11Invoice, error)
	ReceiveVariableAmountViaJitChannel(description Bolt11InvoiceDescription, expirySecs uint32, maxProportionalLspFeeLimitPpmMsat *uint64) (*Bolt11Invoice, error)
	ReceiveVariableAmountViaJitChannelForHash(description Bolt11InvoiceDescription, expirySecs uint32, maxProportionalLspFeeLimitPpmMsat *uint64, paymentHash PaymentHash) (*Bolt11Invoice, error)
	ReceiveViaJitChannel(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, maxLspFeeLimitMsat *uint64) (*Bolt11Invoice, error)
	ReceiveViaJitChannelForHash(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, maxLspFeeLimitMsat *uint64, paymentHash PaymentHash) (*Bolt11Invoice, error)
	Send(invoice *Bolt11Invoice, routeParameters *RouteParametersConfig) (PaymentId, error)
	SendProbes(invoice *Bolt11Invoice, routeParameters *RouteParametersConfig) error
	SendProbesUsingAmount(invoice *Bolt11Invoice, amountMsat uint64, routeParameters *RouteParametersConfig) error
	SendUsingAmount(invoice *Bolt11Invoice, amountMsat uint64, routeParameters *RouteParametersConfig) (PaymentId, error)
	SendWithFirstHop(invoice *Bolt11Invoice, firstHopUserChannelId UserChannelId, routeParameters *RouteParametersConfig) (PaymentId, error)
}
type Bolt11Payment struct {
	ffiObject FfiObject
}

func (_self *Bolt11Payment) ClaimForHash(paymentHash PaymentHash, claimableAmountMsat uint64, preimage PaymentPreimage) error {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_bolt11payment_claim_for_hash(
			_pointer, FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), FfiConverterUint64INSTANCE.Lower(claimableAmountMsat), FfiConverterTypePaymentPreimageINSTANCE.Lower(preimage), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Bolt11Payment) FailForHash(paymentHash PaymentHash) error {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_bolt11payment_fail_for_hash(
			_pointer, FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Bolt11Payment) Receive(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveForHash(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_for_hash(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveForHashWithMinCltvExpiryDelta(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash, minCltvExpiryDelta uint16) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_for_hash_with_min_cltv_expiry_delta(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), FfiConverterUint16INSTANCE.Lower(minCltvExpiryDelta), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveVariableAmount(description Bolt11InvoiceDescription, expirySecs uint32) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_variable_amount(
			_pointer, FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveVariableAmountForHash(description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_variable_amount_for_hash(
			_pointer, FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveVariableAmountForHashWithMinCltvExpiryDelta(description Bolt11InvoiceDescription, expirySecs uint32, paymentHash PaymentHash, minCltvExpiryDelta uint16) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_variable_amount_for_hash_with_min_cltv_expiry_delta(
			_pointer, FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), FfiConverterUint16INSTANCE.Lower(minCltvExpiryDelta), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveVariableAmountViaJitChannel(description Bolt11InvoiceDescription, expirySecs uint32, maxProportionalLspFeeLimitPpmMsat *uint64) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_variable_amount_via_jit_channel(
			_pointer, FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(maxProportionalLspFeeLimitPpmMsat), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveVariableAmountViaJitChannelForHash(description Bolt11InvoiceDescription, expirySecs uint32, maxProportionalLspFeeLimitPpmMsat *uint64, paymentHash PaymentHash) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_variable_amount_via_jit_channel_for_hash(
			_pointer, FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(maxProportionalLspFeeLimitPpmMsat), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveViaJitChannel(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, maxLspFeeLimitMsat *uint64) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_via_jit_channel(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(maxLspFeeLimitMsat), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) ReceiveViaJitChannelForHash(amountMsat uint64, description Bolt11InvoiceDescription, expirySecs uint32, maxLspFeeLimitMsat *uint64, paymentHash PaymentHash) (*Bolt11Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt11payment_receive_via_jit_channel_for_hash(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterBolt11InvoiceDescriptionINSTANCE.Lower(description), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(maxLspFeeLimitMsat), FfiConverterTypePaymentHashINSTANCE.Lower(paymentHash), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt11Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt11InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) Send(invoice *Bolt11Invoice, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11payment_send(
				_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(invoice), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) SendProbes(invoice *Bolt11Invoice, routeParameters *RouteParametersConfig) error {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_bolt11payment_send_probes(
			_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(invoice), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Bolt11Payment) SendProbesUsingAmount(invoice *Bolt11Invoice, amountMsat uint64, routeParameters *RouteParametersConfig) error {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_bolt11payment_send_probes_using_amount(
			_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(invoice), FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Bolt11Payment) SendUsingAmount(invoice *Bolt11Invoice, amountMsat uint64, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11payment_send_using_amount(
				_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(invoice), FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt11Payment) SendWithFirstHop(invoice *Bolt11Invoice, firstHopUserChannelId UserChannelId, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt11Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt11payment_send_with_first_hop(
				_pointer, FfiConverterBolt11InvoiceINSTANCE.Lower(invoice), FfiConverterTypeUserChannelIdINSTANCE.Lower(firstHopUserChannelId), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *Bolt11Payment) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterBolt11Payment struct{}

var FfiConverterBolt11PaymentINSTANCE = FfiConverterBolt11Payment{}

func (c FfiConverterBolt11Payment) Lift(pointer unsafe.Pointer) *Bolt11Payment {
	result := &Bolt11Payment{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_bolt11payment(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_bolt11payment(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Bolt11Payment).Destroy)
	return result
}

func (c FfiConverterBolt11Payment) Read(reader io.Reader) *Bolt11Payment {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterBolt11Payment) Lower(value *Bolt11Payment) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Bolt11Payment")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterBolt11Payment) Write(writer io.Writer, value *Bolt11Payment) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerBolt11Payment struct{}

func (_ FfiDestroyerBolt11Payment) Destroy(value *Bolt11Payment) {
	value.Destroy()
}

type Bolt12InvoiceInterface interface {
	AbsoluteExpirySeconds() *uint64
	Amount() *OfferAmount
	AmountMsats() uint64
	Chain() []uint8
	CreatedAt() uint64
	Encode() []uint8
	FallbackAddresses() []Address
	InvoiceDescription() *string
	IsExpired() bool
	Issuer() *string
	IssuerSigningPubkey() *PublicKey
	Metadata() *[]uint8
	OfferChains() *[][]uint8
	PayerNote() *string
	PayerSigningPubkey() PublicKey
	PaymentHash() PaymentHash
	Quantity() *uint64
	RelativeExpiry() uint64
	SignableHash() []uint8
	SigningPubkey() PublicKey
}
type Bolt12Invoice struct {
	ffiObject FfiObject
}

func Bolt12InvoiceFromStr(invoiceStr string) (*Bolt12Invoice, error) {
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_bolt12invoice_from_str(FfiConverterStringINSTANCE.Lower(invoiceStr), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt12Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt12InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Invoice) AbsoluteExpirySeconds() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_absolute_expiry_seconds(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) Amount() *OfferAmount {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalOfferAmountINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_amount(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) AmountMsats() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt12invoice_amount_msats(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt12Invoice) Chain() []uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_chain(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) CreatedAt() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt12invoice_created_at(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt12Invoice) Encode() []uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_encode(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) FallbackAddresses() []Address {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceTypeAddressINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_fallback_addresses(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) InvoiceDescription() *string {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_invoice_description(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) IsExpired() bool {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_bolt12invoice_is_expired(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt12Invoice) Issuer() *string {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_issuer(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) IssuerSigningPubkey() *PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_issuer_signing_pubkey(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) Metadata() *[]uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_metadata(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) OfferChains() *[][]uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_offer_chains(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) PayerNote() *string {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_payer_note(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) PayerSigningPubkey() PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_payer_signing_pubkey(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) PaymentHash() PaymentHash {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePaymentHashINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_payment_hash(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) Quantity() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_quantity(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) RelativeExpiry() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_bolt12invoice_relative_expiry(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Bolt12Invoice) SignableHash() []uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_signable_hash(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Bolt12Invoice) SigningPubkey() PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Invoice")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12invoice_signing_pubkey(
				_pointer, _uniffiStatus),
		}
	}))
}
func (object *Bolt12Invoice) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterBolt12Invoice struct{}

var FfiConverterBolt12InvoiceINSTANCE = FfiConverterBolt12Invoice{}

func (c FfiConverterBolt12Invoice) Lift(pointer unsafe.Pointer) *Bolt12Invoice {
	result := &Bolt12Invoice{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_bolt12invoice(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_bolt12invoice(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Bolt12Invoice).Destroy)
	return result
}

func (c FfiConverterBolt12Invoice) Read(reader io.Reader) *Bolt12Invoice {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterBolt12Invoice) Lower(value *Bolt12Invoice) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Bolt12Invoice")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterBolt12Invoice) Write(writer io.Writer, value *Bolt12Invoice) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerBolt12Invoice struct{}

func (_ FfiDestroyerBolt12Invoice) Destroy(value *Bolt12Invoice) {
	value.Destroy()
}

type Bolt12PaymentInterface interface {
	BlindedPathsForAsyncRecipient(recipientId []byte) ([]byte, error)
	InitiateRefund(amountMsat uint64, expirySecs uint32, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (*Refund, error)
	Receive(amountMsat uint64, description string, expirySecs *uint32, quantity *uint64) (*Offer, error)
	ReceiveAsync() (*Offer, error)
	ReceiveVariableAmount(description string, expirySecs *uint32) (*Offer, error)
	RequestRefundPayment(refund *Refund) (*Bolt12Invoice, error)
	Send(offer *Offer, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (PaymentId, error)
	SendUsingAmount(offer *Offer, amountMsat uint64, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (PaymentId, error)
	SetPathsToStaticInvoiceServer(paths []byte) error
}
type Bolt12Payment struct {
	ffiObject FfiObject
}

func (_self *Bolt12Payment) BlindedPathsForAsyncRecipient(recipientId []byte) ([]byte, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12payment_blinded_paths_for_async_recipient(
				_pointer, FfiConverterBytesINSTANCE.Lower(recipientId), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue []byte
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBytesINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) InitiateRefund(amountMsat uint64, expirySecs uint32, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (*Refund, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt12payment_initiate_refund(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(quantity), FfiConverterOptionalStringINSTANCE.Lower(payerNote), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Refund
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRefundINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) Receive(amountMsat uint64, description string, expirySecs *uint32, quantity *uint64) (*Offer, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt12payment_receive(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterStringINSTANCE.Lower(description), FfiConverterOptionalUint32INSTANCE.Lower(expirySecs), FfiConverterOptionalUint64INSTANCE.Lower(quantity), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Offer
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOfferINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) ReceiveAsync() (*Offer, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt12payment_receive_async(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Offer
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOfferINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) ReceiveVariableAmount(description string, expirySecs *uint32) (*Offer, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt12payment_receive_variable_amount(
			_pointer, FfiConverterStringINSTANCE.Lower(description), FfiConverterOptionalUint32INSTANCE.Lower(expirySecs), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Offer
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOfferINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) RequestRefundPayment(refund *Refund) (*Bolt12Invoice, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_bolt12payment_request_refund_payment(
			_pointer, FfiConverterRefundINSTANCE.Lower(refund), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Bolt12Invoice
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBolt12InvoiceINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) Send(offer *Offer, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12payment_send(
				_pointer, FfiConverterOfferINSTANCE.Lower(offer), FfiConverterOptionalUint64INSTANCE.Lower(quantity), FfiConverterOptionalStringINSTANCE.Lower(payerNote), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) SendUsingAmount(offer *Offer, amountMsat uint64, quantity *uint64, payerNote *string, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_bolt12payment_send_using_amount(
				_pointer, FfiConverterOfferINSTANCE.Lower(offer), FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterOptionalUint64INSTANCE.Lower(quantity), FfiConverterOptionalStringINSTANCE.Lower(payerNote), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Bolt12Payment) SetPathsToStaticInvoiceServer(paths []byte) error {
	_pointer := _self.ffiObject.incrementPointer("*Bolt12Payment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_bolt12payment_set_paths_to_static_invoice_server(
			_pointer, FfiConverterBytesINSTANCE.Lower(paths), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}
func (object *Bolt12Payment) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterBolt12Payment struct{}

var FfiConverterBolt12PaymentINSTANCE = FfiConverterBolt12Payment{}

func (c FfiConverterBolt12Payment) Lift(pointer unsafe.Pointer) *Bolt12Payment {
	result := &Bolt12Payment{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_bolt12payment(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_bolt12payment(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Bolt12Payment).Destroy)
	return result
}

func (c FfiConverterBolt12Payment) Read(reader io.Reader) *Bolt12Payment {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterBolt12Payment) Lower(value *Bolt12Payment) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Bolt12Payment")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterBolt12Payment) Write(writer io.Writer, value *Bolt12Payment) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerBolt12Payment struct{}

func (_ FfiDestroyerBolt12Payment) Destroy(value *Bolt12Payment) {
	value.Destroy()
}

type BuilderInterface interface {
	Build() (*Node, error)
	BuildWithFsStore() (*Node, error)
	BuildWithVssStore(vssUrl string, storeId string, lnurlAuthServerUrl string, fixedHeaders map[string]string) (*Node, error)
	BuildWithVssStoreAndFixedHeaders(vssUrl string, storeId string, fixedHeaders map[string]string) (*Node, error)
	MigrateStorage(what MigrateStorage)
	ResetState(what ResetState)
	RestoreEncodedChannelMonitors(monitors []KeyValue)
	SetAnnouncementAddresses(announcementAddresses []SocketAddress) error
	SetAsyncPaymentsRole(role *AsyncPaymentsRole) error
	SetChainSourceBitcoindRest(restHost string, restPort uint16, rpcHost string, rpcPort uint16, rpcUser string, rpcPassword string)
	SetChainSourceBitcoindRpc(rpcHost string, rpcPort uint16, rpcUser string, rpcPassword string)
	SetChainSourceElectrum(serverUrl string, config *ElectrumSyncConfig)
	SetChainSourceEsplora(serverUrl string, config *EsploraSyncConfig)
	SetCustomLogger(logWriter LogWriter)
	SetEntropyBip39Mnemonic(mnemonic Mnemonic, passphrase *string)
	SetEntropySeedBytes(seedBytes []uint8) error
	SetEntropySeedPath(seedPath string)
	SetFilesystemLogger(logFilePath *string, maxLogLevel *LogLevel)
	SetGossipSourceP2p()
	SetGossipSourceRgs(rgsServerUrl string)
	SetLiquiditySourceLsps1(nodeId PublicKey, address SocketAddress, token *string)
	SetLiquiditySourceLsps2(nodeId PublicKey, address SocketAddress, token *string)
	SetListeningAddresses(listeningAddresses []SocketAddress) error
	SetLogFacadeLogger()
	SetNetwork(network Network)
	SetNodeAlias(nodeAlias string) error
	SetPathfindingScoresSource(url string)
	SetStorageDirPath(storageDirPath string)
}
type Builder struct {
	ffiObject FfiObject
}

func NewBuilder() *Builder {
	return FfiConverterBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_builder_new(_uniffiStatus)
	}))
}

func BuilderFromConfig(config Config) *Builder {
	return FfiConverterBuilderINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_builder_from_config(FfiConverterConfigINSTANCE.Lower(config), _uniffiStatus)
	}))
}

func (_self *Builder) Build() (*Node, error) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_builder_build(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Node
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterNodeINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Builder) BuildWithFsStore() (*Node, error) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_builder_build_with_fs_store(
			_pointer, _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Node
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterNodeINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Builder) BuildWithVssStore(vssUrl string, storeId string, lnurlAuthServerUrl string, fixedHeaders map[string]string) (*Node, error) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_builder_build_with_vss_store(
			_pointer, FfiConverterStringINSTANCE.Lower(vssUrl), FfiConverterStringINSTANCE.Lower(storeId), FfiConverterStringINSTANCE.Lower(lnurlAuthServerUrl), FfiConverterMapStringStringINSTANCE.Lower(fixedHeaders), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Node
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterNodeINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Builder) BuildWithVssStoreAndFixedHeaders(vssUrl string, storeId string, fixedHeaders map[string]string) (*Node, error) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_builder_build_with_vss_store_and_fixed_headers(
			_pointer, FfiConverterStringINSTANCE.Lower(vssUrl), FfiConverterStringINSTANCE.Lower(storeId), FfiConverterMapStringStringINSTANCE.Lower(fixedHeaders), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Node
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterNodeINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Builder) MigrateStorage(what MigrateStorage) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_migrate_storage(
			_pointer, FfiConverterMigrateStorageINSTANCE.Lower(what), _uniffiStatus)
		return false
	})
}

func (_self *Builder) ResetState(what ResetState) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_reset_state(
			_pointer, FfiConverterResetStateINSTANCE.Lower(what), _uniffiStatus)
		return false
	})
}

func (_self *Builder) RestoreEncodedChannelMonitors(monitors []KeyValue) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_restore_encoded_channel_monitors(
			_pointer, FfiConverterSequenceKeyValueINSTANCE.Lower(monitors), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetAnnouncementAddresses(announcementAddresses []SocketAddress) error {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_announcement_addresses(
			_pointer, FfiConverterSequenceTypeSocketAddressINSTANCE.Lower(announcementAddresses), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Builder) SetAsyncPaymentsRole(role *AsyncPaymentsRole) error {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_async_payments_role(
			_pointer, FfiConverterOptionalAsyncPaymentsRoleINSTANCE.Lower(role), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Builder) SetChainSourceBitcoindRest(restHost string, restPort uint16, rpcHost string, rpcPort uint16, rpcUser string, rpcPassword string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_chain_source_bitcoind_rest(
			_pointer, FfiConverterStringINSTANCE.Lower(restHost), FfiConverterUint16INSTANCE.Lower(restPort), FfiConverterStringINSTANCE.Lower(rpcHost), FfiConverterUint16INSTANCE.Lower(rpcPort), FfiConverterStringINSTANCE.Lower(rpcUser), FfiConverterStringINSTANCE.Lower(rpcPassword), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetChainSourceBitcoindRpc(rpcHost string, rpcPort uint16, rpcUser string, rpcPassword string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_chain_source_bitcoind_rpc(
			_pointer, FfiConverterStringINSTANCE.Lower(rpcHost), FfiConverterUint16INSTANCE.Lower(rpcPort), FfiConverterStringINSTANCE.Lower(rpcUser), FfiConverterStringINSTANCE.Lower(rpcPassword), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetChainSourceElectrum(serverUrl string, config *ElectrumSyncConfig) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_chain_source_electrum(
			_pointer, FfiConverterStringINSTANCE.Lower(serverUrl), FfiConverterOptionalElectrumSyncConfigINSTANCE.Lower(config), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetChainSourceEsplora(serverUrl string, config *EsploraSyncConfig) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_chain_source_esplora(
			_pointer, FfiConverterStringINSTANCE.Lower(serverUrl), FfiConverterOptionalEsploraSyncConfigINSTANCE.Lower(config), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetCustomLogger(logWriter LogWriter) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_custom_logger(
			_pointer, FfiConverterLogWriterINSTANCE.Lower(logWriter), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetEntropyBip39Mnemonic(mnemonic Mnemonic, passphrase *string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_entropy_bip39_mnemonic(
			_pointer, FfiConverterTypeMnemonicINSTANCE.Lower(mnemonic), FfiConverterOptionalStringINSTANCE.Lower(passphrase), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetEntropySeedBytes(seedBytes []uint8) error {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_entropy_seed_bytes(
			_pointer, FfiConverterSequenceUint8INSTANCE.Lower(seedBytes), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Builder) SetEntropySeedPath(seedPath string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_entropy_seed_path(
			_pointer, FfiConverterStringINSTANCE.Lower(seedPath), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetFilesystemLogger(logFilePath *string, maxLogLevel *LogLevel) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_filesystem_logger(
			_pointer, FfiConverterOptionalStringINSTANCE.Lower(logFilePath), FfiConverterOptionalLogLevelINSTANCE.Lower(maxLogLevel), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetGossipSourceP2p() {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_gossip_source_p2p(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetGossipSourceRgs(rgsServerUrl string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_gossip_source_rgs(
			_pointer, FfiConverterStringINSTANCE.Lower(rgsServerUrl), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetLiquiditySourceLsps1(nodeId PublicKey, address SocketAddress, token *string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_liquidity_source_lsps1(
			_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypeSocketAddressINSTANCE.Lower(address), FfiConverterOptionalStringINSTANCE.Lower(token), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetLiquiditySourceLsps2(nodeId PublicKey, address SocketAddress, token *string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_liquidity_source_lsps2(
			_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypeSocketAddressINSTANCE.Lower(address), FfiConverterOptionalStringINSTANCE.Lower(token), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetListeningAddresses(listeningAddresses []SocketAddress) error {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_listening_addresses(
			_pointer, FfiConverterSequenceTypeSocketAddressINSTANCE.Lower(listeningAddresses), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Builder) SetLogFacadeLogger() {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_log_facade_logger(
			_pointer, _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetNetwork(network Network) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_network(
			_pointer, FfiConverterNetworkINSTANCE.Lower(network), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetNodeAlias(nodeAlias string) error {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[BuildError](FfiConverterBuildError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_node_alias(
			_pointer, FfiConverterStringINSTANCE.Lower(nodeAlias), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Builder) SetPathfindingScoresSource(url string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_pathfinding_scores_source(
			_pointer, FfiConverterStringINSTANCE.Lower(url), _uniffiStatus)
		return false
	})
}

func (_self *Builder) SetStorageDirPath(storageDirPath string) {
	_pointer := _self.ffiObject.incrementPointer("*Builder")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_builder_set_storage_dir_path(
			_pointer, FfiConverterStringINSTANCE.Lower(storageDirPath), _uniffiStatus)
		return false
	})
}
func (object *Builder) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterBuilder struct{}

var FfiConverterBuilderINSTANCE = FfiConverterBuilder{}

func (c FfiConverterBuilder) Lift(pointer unsafe.Pointer) *Builder {
	result := &Builder{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_builder(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_builder(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Builder).Destroy)
	return result
}

func (c FfiConverterBuilder) Read(reader io.Reader) *Builder {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterBuilder) Lower(value *Builder) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Builder")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterBuilder) Write(writer io.Writer, value *Builder) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerBuilder struct{}

func (_ FfiDestroyerBuilder) Destroy(value *Builder) {
	value.Destroy()
}

type FeeRateInterface interface {
	ToSatPerKwu() uint64
	ToSatPerVbCeil() uint64
	ToSatPerVbFloor() uint64
}
type FeeRate struct {
	ffiObject FfiObject
}

func FeeRateFromSatPerKwu(satKwu uint64) *FeeRate {
	return FfiConverterFeeRateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_feerate_from_sat_per_kwu(FfiConverterUint64INSTANCE.Lower(satKwu), _uniffiStatus)
	}))
}

func FeeRateFromSatPerVbUnchecked(satVb uint64) *FeeRate {
	return FfiConverterFeeRateINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_feerate_from_sat_per_vb_unchecked(FfiConverterUint64INSTANCE.Lower(satVb), _uniffiStatus)
	}))
}

func (_self *FeeRate) ToSatPerKwu() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*FeeRate")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_feerate_to_sat_per_kwu(
			_pointer, _uniffiStatus)
	}))
}

func (_self *FeeRate) ToSatPerVbCeil() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*FeeRate")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_feerate_to_sat_per_vb_ceil(
			_pointer, _uniffiStatus)
	}))
}

func (_self *FeeRate) ToSatPerVbFloor() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*FeeRate")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_feerate_to_sat_per_vb_floor(
			_pointer, _uniffiStatus)
	}))
}
func (object *FeeRate) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterFeeRate struct{}

var FfiConverterFeeRateINSTANCE = FfiConverterFeeRate{}

func (c FfiConverterFeeRate) Lift(pointer unsafe.Pointer) *FeeRate {
	result := &FeeRate{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_feerate(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_feerate(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*FeeRate).Destroy)
	return result
}

func (c FfiConverterFeeRate) Read(reader io.Reader) *FeeRate {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterFeeRate) Lower(value *FeeRate) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*FeeRate")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterFeeRate) Write(writer io.Writer, value *FeeRate) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerFeeRate struct{}

func (_ FfiDestroyerFeeRate) Destroy(value *FeeRate) {
	value.Destroy()
}

type Lsps1LiquidityInterface interface {
	CheckOrderStatus(orderId LSPS1OrderId) (Lsps1OrderStatus, error)
	RequestChannel(lspBalanceSat uint64, clientBalanceSat uint64, channelExpiryBlocks uint32, announceChannel bool) (Lsps1OrderStatus, error)
}
type Lsps1Liquidity struct {
	ffiObject FfiObject
}

func (_self *Lsps1Liquidity) CheckOrderStatus(orderId LSPS1OrderId) (Lsps1OrderStatus, error) {
	_pointer := _self.ffiObject.incrementPointer("*Lsps1Liquidity")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_lsps1liquidity_check_order_status(
				_pointer, FfiConverterTypeLSPS1OrderIdINSTANCE.Lower(orderId), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Lsps1OrderStatus
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterLsps1OrderStatusINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Lsps1Liquidity) RequestChannel(lspBalanceSat uint64, clientBalanceSat uint64, channelExpiryBlocks uint32, announceChannel bool) (Lsps1OrderStatus, error) {
	_pointer := _self.ffiObject.incrementPointer("*Lsps1Liquidity")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_lsps1liquidity_request_channel(
				_pointer, FfiConverterUint64INSTANCE.Lower(lspBalanceSat), FfiConverterUint64INSTANCE.Lower(clientBalanceSat), FfiConverterUint32INSTANCE.Lower(channelExpiryBlocks), FfiConverterBoolINSTANCE.Lower(announceChannel), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Lsps1OrderStatus
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterLsps1OrderStatusINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *Lsps1Liquidity) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterLsps1Liquidity struct{}

var FfiConverterLsps1LiquidityINSTANCE = FfiConverterLsps1Liquidity{}

func (c FfiConverterLsps1Liquidity) Lift(pointer unsafe.Pointer) *Lsps1Liquidity {
	result := &Lsps1Liquidity{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_lsps1liquidity(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_lsps1liquidity(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Lsps1Liquidity).Destroy)
	return result
}

func (c FfiConverterLsps1Liquidity) Read(reader io.Reader) *Lsps1Liquidity {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterLsps1Liquidity) Lower(value *Lsps1Liquidity) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Lsps1Liquidity")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterLsps1Liquidity) Write(writer io.Writer, value *Lsps1Liquidity) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerLsps1Liquidity struct{}

func (_ FfiDestroyerLsps1Liquidity) Destroy(value *Lsps1Liquidity) {
	value.Destroy()
}

type Lsps2LiquidityInterface interface {
	RequestOpeningFeeParams() (Lsps2GetInfoResponse, error)
}
type Lsps2Liquidity struct {
	ffiObject FfiObject
}

func (_self *Lsps2Liquidity) RequestOpeningFeeParams() (Lsps2GetInfoResponse, error) {
	_pointer := _self.ffiObject.incrementPointer("*Lsps2Liquidity")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_lsps2liquidity_request_opening_fee_params(
				_pointer, _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Lsps2GetInfoResponse
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterLsps2GetInfoResponseINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *Lsps2Liquidity) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterLsps2Liquidity struct{}

var FfiConverterLsps2LiquidityINSTANCE = FfiConverterLsps2Liquidity{}

func (c FfiConverterLsps2Liquidity) Lift(pointer unsafe.Pointer) *Lsps2Liquidity {
	result := &Lsps2Liquidity{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_lsps2liquidity(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_lsps2liquidity(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Lsps2Liquidity).Destroy)
	return result
}

func (c FfiConverterLsps2Liquidity) Read(reader io.Reader) *Lsps2Liquidity {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterLsps2Liquidity) Lower(value *Lsps2Liquidity) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Lsps2Liquidity")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterLsps2Liquidity) Write(writer io.Writer, value *Lsps2Liquidity) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerLsps2Liquidity struct{}

func (_ FfiDestroyerLsps2Liquidity) Destroy(value *Lsps2Liquidity) {
	value.Destroy()
}

type LogWriter interface {
	Log(record LogRecord)
}
type LogWriterImpl struct {
	ffiObject FfiObject
}

func (_self *LogWriterImpl) Log(record LogRecord) {
	_pointer := _self.ffiObject.incrementPointer("LogWriter")
	defer _self.ffiObject.decrementPointer()
	rustCall(func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_logwriter_log(
			_pointer, FfiConverterLogRecordINSTANCE.Lower(record), _uniffiStatus)
		return false
	})
}
func (object *LogWriterImpl) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterLogWriter struct {
	handleMap *concurrentHandleMap[LogWriter]
}

var FfiConverterLogWriterINSTANCE = FfiConverterLogWriter{
	handleMap: newConcurrentHandleMap[LogWriter](),
}

func (c FfiConverterLogWriter) Lift(pointer unsafe.Pointer) LogWriter {
	result := &LogWriterImpl{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_logwriter(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_logwriter(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*LogWriterImpl).Destroy)
	return result
}

func (c FfiConverterLogWriter) Read(reader io.Reader) LogWriter {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterLogWriter) Lower(value LogWriter) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := unsafe.Pointer(uintptr(c.handleMap.insert(value)))
	return pointer

}

func (c FfiConverterLogWriter) Write(writer io.Writer, value LogWriter) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerLogWriter struct{}

func (_ FfiDestroyerLogWriter) Destroy(value LogWriter) {
	if val, ok := value.(*LogWriterImpl); ok {
		val.Destroy()
	} else {
		panic("Expected *LogWriterImpl")
	}
}

type uniffiCallbackResult C.int8_t

const (
	uniffiIdxCallbackFree               uniffiCallbackResult = 0
	uniffiCallbackResultSuccess         uniffiCallbackResult = 0
	uniffiCallbackResultError           uniffiCallbackResult = 1
	uniffiCallbackUnexpectedResultError uniffiCallbackResult = 2
	uniffiCallbackCancelled             uniffiCallbackResult = 3
)

type concurrentHandleMap[T any] struct {
	handles       map[uint64]T
	currentHandle uint64
	lock          sync.RWMutex
}

func newConcurrentHandleMap[T any]() *concurrentHandleMap[T] {
	return &concurrentHandleMap[T]{
		handles: map[uint64]T{},
	}
}

func (cm *concurrentHandleMap[T]) insert(obj T) uint64 {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	cm.currentHandle = cm.currentHandle + 1
	cm.handles[cm.currentHandle] = obj
	return cm.currentHandle
}

func (cm *concurrentHandleMap[T]) remove(handle uint64) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	delete(cm.handles, handle)
}

func (cm *concurrentHandleMap[T]) tryGet(handle uint64) (T, bool) {
	cm.lock.RLock()
	defer cm.lock.RUnlock()

	val, ok := cm.handles[handle]
	return val, ok
}

//export ldk_node_cgo_dispatchCallbackInterfaceLogWriterMethod0
func ldk_node_cgo_dispatchCallbackInterfaceLogWriterMethod0(uniffiHandle C.uint64_t, record C.RustBuffer, uniffiOutReturn *C.void, callStatus *C.RustCallStatus) {
	handle := uint64(uniffiHandle)
	uniffiObj, ok := FfiConverterLogWriterINSTANCE.handleMap.tryGet(handle)
	if !ok {
		panic(fmt.Errorf("no callback in handle map: %d", handle))
	}

	uniffiObj.Log(
		FfiConverterLogRecordINSTANCE.Lift(GoRustBuffer{
			inner: record,
		}),
	)

}

var UniffiVTableCallbackInterfaceLogWriterINSTANCE = C.UniffiVTableCallbackInterfaceLogWriter{
	log: (C.UniffiCallbackInterfaceLogWriterMethod0)(C.ldk_node_cgo_dispatchCallbackInterfaceLogWriterMethod0),

	uniffiFree: (C.UniffiCallbackInterfaceFree)(C.ldk_node_cgo_dispatchCallbackInterfaceLogWriterFree),
}

//export ldk_node_cgo_dispatchCallbackInterfaceLogWriterFree
func ldk_node_cgo_dispatchCallbackInterfaceLogWriterFree(handle C.uint64_t) {
	FfiConverterLogWriterINSTANCE.handleMap.remove(uint64(handle))
}

func (c FfiConverterLogWriter) register() {
	C.uniffi_ldk_node_fn_init_callback_vtable_logwriter(&UniffiVTableCallbackInterfaceLogWriterINSTANCE)
}

type NetworkGraphInterface interface {
	Channel(shortChannelId uint64) *ChannelInfo
	ListChannels() []uint64
	ListNodes() []NodeId
	Node(nodeId NodeId) *NodeInfo
}
type NetworkGraph struct {
	ffiObject FfiObject
}

func (_self *NetworkGraph) Channel(shortChannelId uint64) *ChannelInfo {
	_pointer := _self.ffiObject.incrementPointer("*NetworkGraph")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalChannelInfoINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_networkgraph_channel(
				_pointer, FfiConverterUint64INSTANCE.Lower(shortChannelId), _uniffiStatus),
		}
	}))
}

func (_self *NetworkGraph) ListChannels() []uint64 {
	_pointer := _self.ffiObject.incrementPointer("*NetworkGraph")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_networkgraph_list_channels(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *NetworkGraph) ListNodes() []NodeId {
	_pointer := _self.ffiObject.incrementPointer("*NetworkGraph")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceTypeNodeIdINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_networkgraph_list_nodes(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *NetworkGraph) Node(nodeId NodeId) *NodeInfo {
	_pointer := _self.ffiObject.incrementPointer("*NetworkGraph")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalNodeInfoINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_networkgraph_node(
				_pointer, FfiConverterTypeNodeIdINSTANCE.Lower(nodeId), _uniffiStatus),
		}
	}))
}
func (object *NetworkGraph) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterNetworkGraph struct{}

var FfiConverterNetworkGraphINSTANCE = FfiConverterNetworkGraph{}

func (c FfiConverterNetworkGraph) Lift(pointer unsafe.Pointer) *NetworkGraph {
	result := &NetworkGraph{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_networkgraph(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_networkgraph(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*NetworkGraph).Destroy)
	return result
}

func (c FfiConverterNetworkGraph) Read(reader io.Reader) *NetworkGraph {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterNetworkGraph) Lower(value *NetworkGraph) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*NetworkGraph")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterNetworkGraph) Write(writer io.Writer, value *NetworkGraph) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerNetworkGraph struct{}

func (_ FfiDestroyerNetworkGraph) Destroy(value *NetworkGraph) {
	value.Destroy()
}

type NodeInterface interface {
	AnnouncementAddresses() *[]SocketAddress
	Bolt11Payment() *Bolt11Payment
	Bolt12Payment() *Bolt12Payment
	CloseChannel(userChannelId UserChannelId, counterpartyNodeId PublicKey) error
	Config() Config
	Connect(nodeId PublicKey, address SocketAddress, persist bool) error
	Disconnect(nodeId PublicKey) error
	EventHandled() error
	ExportPathfindingScores() ([]byte, error)
	ForceCloseChannel(userChannelId UserChannelId, counterpartyNodeId PublicKey, reason *string) error
	GetEncodedChannelMonitors() ([]KeyValue, error)
	ListBalances() BalanceDetails
	ListChannelMonitorSizes() []ChannelMonitorSizeInfo
	ListChannels() []ChannelDetails
	ListPayments() []PaymentDetails
	ListPeers() []PeerDetails
	ListeningAddresses() *[]SocketAddress
	Lsps1Liquidity() *Lsps1Liquidity
	Lsps2Liquidity() *Lsps2Liquidity
	NetworkGraph() *NetworkGraph
	NextEvent() *Event
	NextEventAsync() Event
	NodeAlias() *NodeAlias
	NodeId() PublicKey
	OnchainPayment() *OnchainPayment
	OpenAnnouncedChannel(nodeId PublicKey, address SocketAddress, channelAmountSats uint64, pushToCounterpartyMsat *uint64, channelConfig *ChannelConfig) (UserChannelId, error)
	OpenChannel(nodeId PublicKey, address SocketAddress, channelAmountSats uint64, pushToCounterpartyMsat *uint64, channelConfig *ChannelConfig) (UserChannelId, error)
	Payment(paymentId PaymentId) *PaymentDetails
	RemovePayment(paymentId PaymentId) error
	SignMessage(msg []uint8) string
	SpliceIn(userChannelId UserChannelId, counterpartyNodeId PublicKey, spliceAmountSats uint64) error
	SpliceOut(userChannelId UserChannelId, counterpartyNodeId PublicKey, address Address, spliceAmountSats uint64) error
	SpontaneousPayment() *SpontaneousPayment
	Start() error
	Status() NodeStatus
	Stop() error
	SyncWallets() error
	UnifiedQrPayment() *UnifiedQrPayment
	UpdateChannelConfig(userChannelId UserChannelId, counterpartyNodeId PublicKey, channelConfig ChannelConfig) error
	UpdateFeeEstimates() error
	VerifySignature(msg []uint8, sig string, pkey PublicKey) bool
	WaitNextEvent() Event
}
type Node struct {
	ffiObject FfiObject
}

func (_self *Node) AnnouncementAddresses() *[]SocketAddress {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_announcement_addresses(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) Bolt11Payment() *Bolt11Payment {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBolt11PaymentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_bolt11_payment(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) Bolt12Payment() *Bolt12Payment {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBolt12PaymentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_bolt12_payment(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) CloseChannel(userChannelId UserChannelId, counterpartyNodeId PublicKey) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_close_channel(
			_pointer, FfiConverterTypeUserChannelIdINSTANCE.Lower(userChannelId), FfiConverterTypePublicKeyINSTANCE.Lower(counterpartyNodeId), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) Config() Config {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterConfigINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_config(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) Connect(nodeId PublicKey, address SocketAddress, persist bool) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_connect(
			_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypeSocketAddressINSTANCE.Lower(address), FfiConverterBoolINSTANCE.Lower(persist), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) Disconnect(nodeId PublicKey) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_disconnect(
			_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) EventHandled() error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_event_handled(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) ExportPathfindingScores() ([]byte, error) {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_export_pathfinding_scores(
				_pointer, _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue []byte
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterBytesINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Node) ForceCloseChannel(userChannelId UserChannelId, counterpartyNodeId PublicKey, reason *string) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_force_close_channel(
			_pointer, FfiConverterTypeUserChannelIdINSTANCE.Lower(userChannelId), FfiConverterTypePublicKeyINSTANCE.Lower(counterpartyNodeId), FfiConverterOptionalStringINSTANCE.Lower(reason), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) GetEncodedChannelMonitors() ([]KeyValue, error) {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_get_encoded_channel_monitors(
				_pointer, _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue []KeyValue
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterSequenceKeyValueINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Node) ListBalances() BalanceDetails {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBalanceDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_list_balances(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) ListChannelMonitorSizes() []ChannelMonitorSizeInfo {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceChannelMonitorSizeInfoINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_list_channel_monitor_sizes(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) ListChannels() []ChannelDetails {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceChannelDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_list_channels(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) ListPayments() []PaymentDetails {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequencePaymentDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_list_payments(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) ListPeers() []PeerDetails {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequencePeerDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_list_peers(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) ListeningAddresses() *[]SocketAddress {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_listening_addresses(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) Lsps1Liquidity() *Lsps1Liquidity {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterLsps1LiquidityINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_lsps1_liquidity(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) Lsps2Liquidity() *Lsps2Liquidity {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterLsps2LiquidityINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_lsps2_liquidity(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) NetworkGraph() *NetworkGraph {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNetworkGraphINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_network_graph(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) NextEvent() *Event {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalEventINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_next_event(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) NextEventAsync() Event {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	res, _ := uniffiRustCallAsync[error](
		nil,
		// completeFn
		func(handle C.uint64_t, status *C.RustCallStatus) RustBufferI {
			res := C.ffi_ldk_node_rust_future_complete_rust_buffer(handle, status)
			return GoRustBuffer{
				inner: res,
			}
		},
		// liftFn
		func(ffi RustBufferI) Event {
			return FfiConverterEventINSTANCE.Lift(ffi)
		},
		C.uniffi_ldk_node_fn_method_node_next_event_async(
			_pointer),
		// pollFn
		func(handle C.uint64_t, continuation C.UniffiRustFutureContinuationCallback, data C.uint64_t) {
			C.ffi_ldk_node_rust_future_poll_rust_buffer(handle, continuation, data)
		},
		// freeFn
		func(handle C.uint64_t) {
			C.ffi_ldk_node_rust_future_free_rust_buffer(handle)
		},
	)

	return res
}

func (_self *Node) NodeAlias() *NodeAlias {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypeNodeAliasINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_node_alias(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) NodeId() PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_node_id(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) OnchainPayment() *OnchainPayment {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOnchainPaymentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_onchain_payment(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) OpenAnnouncedChannel(nodeId PublicKey, address SocketAddress, channelAmountSats uint64, pushToCounterpartyMsat *uint64, channelConfig *ChannelConfig) (UserChannelId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_open_announced_channel(
				_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypeSocketAddressINSTANCE.Lower(address), FfiConverterUint64INSTANCE.Lower(channelAmountSats), FfiConverterOptionalUint64INSTANCE.Lower(pushToCounterpartyMsat), FfiConverterOptionalChannelConfigINSTANCE.Lower(channelConfig), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue UserChannelId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeUserChannelIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Node) OpenChannel(nodeId PublicKey, address SocketAddress, channelAmountSats uint64, pushToCounterpartyMsat *uint64, channelConfig *ChannelConfig) (UserChannelId, error) {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_open_channel(
				_pointer, FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypeSocketAddressINSTANCE.Lower(address), FfiConverterUint64INSTANCE.Lower(channelAmountSats), FfiConverterOptionalUint64INSTANCE.Lower(pushToCounterpartyMsat), FfiConverterOptionalChannelConfigINSTANCE.Lower(channelConfig), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue UserChannelId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeUserChannelIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Node) Payment(paymentId PaymentId) *PaymentDetails {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalPaymentDetailsINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_payment(
				_pointer, FfiConverterTypePaymentIdINSTANCE.Lower(paymentId), _uniffiStatus),
		}
	}))
}

func (_self *Node) RemovePayment(paymentId PaymentId) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_remove_payment(
			_pointer, FfiConverterTypePaymentIdINSTANCE.Lower(paymentId), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) SignMessage(msg []uint8) string {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_sign_message(
				_pointer, FfiConverterSequenceUint8INSTANCE.Lower(msg), _uniffiStatus),
		}
	}))
}

func (_self *Node) SpliceIn(userChannelId UserChannelId, counterpartyNodeId PublicKey, spliceAmountSats uint64) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_splice_in(
			_pointer, FfiConverterTypeUserChannelIdINSTANCE.Lower(userChannelId), FfiConverterTypePublicKeyINSTANCE.Lower(counterpartyNodeId), FfiConverterUint64INSTANCE.Lower(spliceAmountSats), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) SpliceOut(userChannelId UserChannelId, counterpartyNodeId PublicKey, address Address, spliceAmountSats uint64) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_splice_out(
			_pointer, FfiConverterTypeUserChannelIdINSTANCE.Lower(userChannelId), FfiConverterTypePublicKeyINSTANCE.Lower(counterpartyNodeId), FfiConverterTypeAddressINSTANCE.Lower(address), FfiConverterUint64INSTANCE.Lower(spliceAmountSats), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) SpontaneousPayment() *SpontaneousPayment {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSpontaneousPaymentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_spontaneous_payment(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) Start() error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_start(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) Status() NodeStatus {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterNodeStatusINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_status(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Node) Stop() error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_stop(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) SyncWallets() error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_sync_wallets(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) UnifiedQrPayment() *UnifiedQrPayment {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUnifiedQrPaymentINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_method_node_unified_qr_payment(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Node) UpdateChannelConfig(userChannelId UserChannelId, counterpartyNodeId PublicKey, channelConfig ChannelConfig) error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_update_channel_config(
			_pointer, FfiConverterTypeUserChannelIdINSTANCE.Lower(userChannelId), FfiConverterTypePublicKeyINSTANCE.Lower(counterpartyNodeId), FfiConverterChannelConfigINSTANCE.Lower(channelConfig), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) UpdateFeeEstimates() error {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_node_update_fee_estimates(
			_pointer, _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *Node) VerifySignature(msg []uint8, sig string, pkey PublicKey) bool {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_node_verify_signature(
			_pointer, FfiConverterSequenceUint8INSTANCE.Lower(msg), FfiConverterStringINSTANCE.Lower(sig), FfiConverterTypePublicKeyINSTANCE.Lower(pkey), _uniffiStatus)
	}))
}

func (_self *Node) WaitNextEvent() Event {
	_pointer := _self.ffiObject.incrementPointer("*Node")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterEventINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_node_wait_next_event(
				_pointer, _uniffiStatus),
		}
	}))
}
func (object *Node) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterNode struct{}

var FfiConverterNodeINSTANCE = FfiConverterNode{}

func (c FfiConverterNode) Lift(pointer unsafe.Pointer) *Node {
	result := &Node{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_node(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_node(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Node).Destroy)
	return result
}

func (c FfiConverterNode) Read(reader io.Reader) *Node {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterNode) Lower(value *Node) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Node")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterNode) Write(writer io.Writer, value *Node) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerNode struct{}

func (_ FfiDestroyerNode) Destroy(value *Node) {
	value.Destroy()
}

type OfferInterface interface {
	AbsoluteExpirySeconds() *uint64
	Amount() *OfferAmount
	Chains() []Network
	ExpectsQuantity() bool
	Id() OfferId
	IsExpired() bool
	IsValidQuantity(quantity uint64) bool
	Issuer() *string
	IssuerSigningPubkey() *PublicKey
	Metadata() *[]uint8
	OfferDescription() *string
	SupportsChain(chain Network) bool
}
type Offer struct {
	ffiObject FfiObject
}

func OfferFromStr(offerStr string) (*Offer, error) {
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_offer_from_str(FfiConverterStringINSTANCE.Lower(offerStr), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Offer
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterOfferINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Offer) AbsoluteExpirySeconds() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_absolute_expiry_seconds(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) Amount() *OfferAmount {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalOfferAmountINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_amount(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) Chains() []Network {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceNetworkINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_chains(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) ExpectsQuantity() bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_expects_quantity(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Offer) Id() OfferId {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypeOfferIdINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_id(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) IsExpired() bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_is_expired(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Offer) IsValidQuantity(quantity uint64) bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_is_valid_quantity(
			_pointer, FfiConverterUint64INSTANCE.Lower(quantity), _uniffiStatus)
	}))
}

func (_self *Offer) Issuer() *string {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_issuer(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) IssuerSigningPubkey() *PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_issuer_signing_pubkey(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) Metadata() *[]uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_metadata(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) OfferDescription() *string {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_offer_description(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) SupportsChain(chain Network) bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_supports_chain(
			_pointer, FfiConverterNetworkINSTANCE.Lower(chain), _uniffiStatus)
	}))
}

func (_self *Offer) DebugString() string {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_uniffi_trait_debug(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) String() string {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_offer_uniffi_trait_display(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Offer) Eq(other *Offer) bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_uniffi_trait_eq_eq(
			_pointer, FfiConverterOfferINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (_self *Offer) Ne(other *Offer) bool {
	_pointer := _self.ffiObject.incrementPointer("*Offer")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_offer_uniffi_trait_eq_ne(
			_pointer, FfiConverterOfferINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (object *Offer) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterOffer struct{}

var FfiConverterOfferINSTANCE = FfiConverterOffer{}

func (c FfiConverterOffer) Lift(pointer unsafe.Pointer) *Offer {
	result := &Offer{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_offer(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_offer(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Offer).Destroy)
	return result
}

func (c FfiConverterOffer) Read(reader io.Reader) *Offer {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterOffer) Lower(value *Offer) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Offer")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterOffer) Write(writer io.Writer, value *Offer) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerOffer struct{}

func (_ FfiDestroyerOffer) Destroy(value *Offer) {
	value.Destroy()
}

type OnchainPaymentInterface interface {
	NewAddress() (Address, error)
	SendAllToAddress(address Address, retainReserve bool, feeRate **FeeRate) (Txid, error)
	SendToAddress(address Address, amountSats uint64, feeRate **FeeRate) (Txid, error)
}
type OnchainPayment struct {
	ffiObject FfiObject
}

func (_self *OnchainPayment) NewAddress() (Address, error) {
	_pointer := _self.ffiObject.incrementPointer("*OnchainPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_onchainpayment_new_address(
				_pointer, _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Address
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeAddressINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *OnchainPayment) SendAllToAddress(address Address, retainReserve bool, feeRate **FeeRate) (Txid, error) {
	_pointer := _self.ffiObject.incrementPointer("*OnchainPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_onchainpayment_send_all_to_address(
				_pointer, FfiConverterTypeAddressINSTANCE.Lower(address), FfiConverterBoolINSTANCE.Lower(retainReserve), FfiConverterOptionalFeeRateINSTANCE.Lower(feeRate), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Txid
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeTxidINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *OnchainPayment) SendToAddress(address Address, amountSats uint64, feeRate **FeeRate) (Txid, error) {
	_pointer := _self.ffiObject.incrementPointer("*OnchainPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_onchainpayment_send_to_address(
				_pointer, FfiConverterTypeAddressINSTANCE.Lower(address), FfiConverterUint64INSTANCE.Lower(amountSats), FfiConverterOptionalFeeRateINSTANCE.Lower(feeRate), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue Txid
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypeTxidINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *OnchainPayment) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterOnchainPayment struct{}

var FfiConverterOnchainPaymentINSTANCE = FfiConverterOnchainPayment{}

func (c FfiConverterOnchainPayment) Lift(pointer unsafe.Pointer) *OnchainPayment {
	result := &OnchainPayment{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_onchainpayment(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_onchainpayment(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*OnchainPayment).Destroy)
	return result
}

func (c FfiConverterOnchainPayment) Read(reader io.Reader) *OnchainPayment {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterOnchainPayment) Lower(value *OnchainPayment) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*OnchainPayment")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterOnchainPayment) Write(writer io.Writer, value *OnchainPayment) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerOnchainPayment struct{}

func (_ FfiDestroyerOnchainPayment) Destroy(value *OnchainPayment) {
	value.Destroy()
}

type RefundInterface interface {
	AbsoluteExpirySeconds() *uint64
	AmountMsats() uint64
	Chain() *Network
	IsExpired() bool
	Issuer() *string
	PayerMetadata() []uint8
	PayerNote() *string
	PayerSigningPubkey() PublicKey
	Quantity() *uint64
	RefundDescription() string
}
type Refund struct {
	ffiObject FfiObject
}

func RefundFromStr(refundStr string) (*Refund, error) {
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) unsafe.Pointer {
		return C.uniffi_ldk_node_fn_constructor_refund_from_str(FfiConverterStringINSTANCE.Lower(refundStr), _uniffiStatus)
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue *Refund
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterRefundINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *Refund) AbsoluteExpirySeconds() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_absolute_expiry_seconds(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) AmountMsats() uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.uint64_t {
		return C.uniffi_ldk_node_fn_method_refund_amount_msats(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Refund) Chain() *Network {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalNetworkINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_chain(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) IsExpired() bool {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_refund_is_expired(
			_pointer, _uniffiStatus)
	}))
}

func (_self *Refund) Issuer() *string {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_issuer(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) PayerMetadata() []uint8 {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterSequenceUint8INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_payer_metadata(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) PayerNote() *string {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_payer_note(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) PayerSigningPubkey() PublicKey {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterTypePublicKeyINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_payer_signing_pubkey(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) Quantity() *uint64 {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_quantity(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) RefundDescription() string {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_refund_description(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) DebugString() string {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_uniffi_trait_debug(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) String() string {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterStringINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_refund_uniffi_trait_display(
				_pointer, _uniffiStatus),
		}
	}))
}

func (_self *Refund) Eq(other *Refund) bool {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_refund_uniffi_trait_eq_eq(
			_pointer, FfiConverterRefundINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (_self *Refund) Ne(other *Refund) bool {
	_pointer := _self.ffiObject.incrementPointer("*Refund")
	defer _self.ffiObject.decrementPointer()
	return FfiConverterBoolINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) C.int8_t {
		return C.uniffi_ldk_node_fn_method_refund_uniffi_trait_eq_ne(
			_pointer, FfiConverterRefundINSTANCE.Lower(other), _uniffiStatus)
	}))
}

func (object *Refund) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterRefund struct{}

var FfiConverterRefundINSTANCE = FfiConverterRefund{}

func (c FfiConverterRefund) Lift(pointer unsafe.Pointer) *Refund {
	result := &Refund{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_refund(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_refund(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*Refund).Destroy)
	return result
}

func (c FfiConverterRefund) Read(reader io.Reader) *Refund {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterRefund) Lower(value *Refund) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*Refund")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterRefund) Write(writer io.Writer, value *Refund) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerRefund struct{}

func (_ FfiDestroyerRefund) Destroy(value *Refund) {
	value.Destroy()
}

type SpontaneousPaymentInterface interface {
	Send(amountMsat uint64, nodeId PublicKey, routeParameters *RouteParametersConfig) (PaymentId, error)
	SendProbes(amountMsat uint64, nodeId PublicKey) error
	SendWithCustomTlvs(amountMsat uint64, nodeId PublicKey, routeParameters *RouteParametersConfig, customTlvs []CustomTlvRecord) (PaymentId, error)
	SendWithPreimage(amountMsat uint64, nodeId PublicKey, preimage PaymentPreimage, routeParameters *RouteParametersConfig) (PaymentId, error)
	SendWithPreimageAndCustomTlvs(amountMsat uint64, nodeId PublicKey, customTlvs []CustomTlvRecord, preimage PaymentPreimage, routeParameters *RouteParametersConfig) (PaymentId, error)
}
type SpontaneousPayment struct {
	ffiObject FfiObject
}

func (_self *SpontaneousPayment) Send(amountMsat uint64, nodeId PublicKey, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*SpontaneousPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_spontaneouspayment_send(
				_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *SpontaneousPayment) SendProbes(amountMsat uint64, nodeId PublicKey) error {
	_pointer := _self.ffiObject.incrementPointer("*SpontaneousPayment")
	defer _self.ffiObject.decrementPointer()
	_, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) bool {
		C.uniffi_ldk_node_fn_method_spontaneouspayment_send_probes(
			_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), _uniffiStatus)
		return false
	})
	return _uniffiErr.AsError()
}

func (_self *SpontaneousPayment) SendWithCustomTlvs(amountMsat uint64, nodeId PublicKey, routeParameters *RouteParametersConfig, customTlvs []CustomTlvRecord) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*SpontaneousPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_spontaneouspayment_send_with_custom_tlvs(
				_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), FfiConverterSequenceCustomTlvRecordINSTANCE.Lower(customTlvs), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *SpontaneousPayment) SendWithPreimage(amountMsat uint64, nodeId PublicKey, preimage PaymentPreimage, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*SpontaneousPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_spontaneouspayment_send_with_preimage(
				_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterTypePaymentPreimageINSTANCE.Lower(preimage), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *SpontaneousPayment) SendWithPreimageAndCustomTlvs(amountMsat uint64, nodeId PublicKey, customTlvs []CustomTlvRecord, preimage PaymentPreimage, routeParameters *RouteParametersConfig) (PaymentId, error) {
	_pointer := _self.ffiObject.incrementPointer("*SpontaneousPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_spontaneouspayment_send_with_preimage_and_custom_tlvs(
				_pointer, FfiConverterUint64INSTANCE.Lower(amountMsat), FfiConverterTypePublicKeyINSTANCE.Lower(nodeId), FfiConverterSequenceCustomTlvRecordINSTANCE.Lower(customTlvs), FfiConverterTypePaymentPreimageINSTANCE.Lower(preimage), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue PaymentId
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterTypePaymentIdINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *SpontaneousPayment) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterSpontaneousPayment struct{}

var FfiConverterSpontaneousPaymentINSTANCE = FfiConverterSpontaneousPayment{}

func (c FfiConverterSpontaneousPayment) Lift(pointer unsafe.Pointer) *SpontaneousPayment {
	result := &SpontaneousPayment{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_spontaneouspayment(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_spontaneouspayment(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*SpontaneousPayment).Destroy)
	return result
}

func (c FfiConverterSpontaneousPayment) Read(reader io.Reader) *SpontaneousPayment {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterSpontaneousPayment) Lower(value *SpontaneousPayment) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*SpontaneousPayment")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterSpontaneousPayment) Write(writer io.Writer, value *SpontaneousPayment) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerSpontaneousPayment struct{}

func (_ FfiDestroyerSpontaneousPayment) Destroy(value *SpontaneousPayment) {
	value.Destroy()
}

type UnifiedQrPaymentInterface interface {
	Receive(amountSats uint64, message string, expirySec uint32) (string, error)
	Send(uriStr string, routeParameters *RouteParametersConfig) (QrPaymentResult, error)
}
type UnifiedQrPayment struct {
	ffiObject FfiObject
}

func (_self *UnifiedQrPayment) Receive(amountSats uint64, message string, expirySec uint32) (string, error) {
	_pointer := _self.ffiObject.incrementPointer("*UnifiedQrPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_unifiedqrpayment_receive(
				_pointer, FfiConverterUint64INSTANCE.Lower(amountSats), FfiConverterStringINSTANCE.Lower(message), FfiConverterUint32INSTANCE.Lower(expirySec), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue string
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterStringINSTANCE.Lift(_uniffiRV), nil
	}
}

func (_self *UnifiedQrPayment) Send(uriStr string, routeParameters *RouteParametersConfig) (QrPaymentResult, error) {
	_pointer := _self.ffiObject.incrementPointer("*UnifiedQrPayment")
	defer _self.ffiObject.decrementPointer()
	_uniffiRV, _uniffiErr := rustCallWithError[NodeError](FfiConverterNodeError{}, func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_method_unifiedqrpayment_send(
				_pointer, FfiConverterStringINSTANCE.Lower(uriStr), FfiConverterOptionalRouteParametersConfigINSTANCE.Lower(routeParameters), _uniffiStatus),
		}
	})
	if _uniffiErr != nil {
		var _uniffiDefaultValue QrPaymentResult
		return _uniffiDefaultValue, _uniffiErr
	} else {
		return FfiConverterQrPaymentResultINSTANCE.Lift(_uniffiRV), nil
	}
}
func (object *UnifiedQrPayment) Destroy() {
	runtime.SetFinalizer(object, nil)
	object.ffiObject.destroy()
}

type FfiConverterUnifiedQrPayment struct{}

var FfiConverterUnifiedQrPaymentINSTANCE = FfiConverterUnifiedQrPayment{}

func (c FfiConverterUnifiedQrPayment) Lift(pointer unsafe.Pointer) *UnifiedQrPayment {
	result := &UnifiedQrPayment{
		newFfiObject(
			pointer,
			func(pointer unsafe.Pointer, status *C.RustCallStatus) unsafe.Pointer {
				return C.uniffi_ldk_node_fn_clone_unifiedqrpayment(pointer, status)
			},
			func(pointer unsafe.Pointer, status *C.RustCallStatus) {
				C.uniffi_ldk_node_fn_free_unifiedqrpayment(pointer, status)
			},
		),
	}
	runtime.SetFinalizer(result, (*UnifiedQrPayment).Destroy)
	return result
}

func (c FfiConverterUnifiedQrPayment) Read(reader io.Reader) *UnifiedQrPayment {
	return c.Lift(unsafe.Pointer(uintptr(readUint64(reader))))
}

func (c FfiConverterUnifiedQrPayment) Lower(value *UnifiedQrPayment) unsafe.Pointer {
	// TODO: this is bad - all synchronization from ObjectRuntime.go is discarded here,
	// because the pointer will be decremented immediately after this function returns,
	// and someone will be left holding onto a non-locked pointer.
	pointer := value.ffiObject.incrementPointer("*UnifiedQrPayment")
	defer value.ffiObject.decrementPointer()
	return pointer

}

func (c FfiConverterUnifiedQrPayment) Write(writer io.Writer, value *UnifiedQrPayment) {
	writeUint64(writer, uint64(uintptr(c.Lower(value))))
}

type FfiDestroyerUnifiedQrPayment struct{}

func (_ FfiDestroyerUnifiedQrPayment) Destroy(value *UnifiedQrPayment) {
	value.Destroy()
}

type AnchorChannelsConfig struct {
	TrustedPeersNoReserve []PublicKey
	PerChannelReserveSats uint64
}

func (r *AnchorChannelsConfig) Destroy() {
	FfiDestroyerSequenceTypePublicKey{}.Destroy(r.TrustedPeersNoReserve)
	FfiDestroyerUint64{}.Destroy(r.PerChannelReserveSats)
}

type FfiConverterAnchorChannelsConfig struct{}

var FfiConverterAnchorChannelsConfigINSTANCE = FfiConverterAnchorChannelsConfig{}

func (c FfiConverterAnchorChannelsConfig) Lift(rb RustBufferI) AnchorChannelsConfig {
	return LiftFromRustBuffer[AnchorChannelsConfig](c, rb)
}

func (c FfiConverterAnchorChannelsConfig) Read(reader io.Reader) AnchorChannelsConfig {
	return AnchorChannelsConfig{
		FfiConverterSequenceTypePublicKeyINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterAnchorChannelsConfig) Lower(value AnchorChannelsConfig) C.RustBuffer {
	return LowerIntoRustBuffer[AnchorChannelsConfig](c, value)
}

func (c FfiConverterAnchorChannelsConfig) Write(writer io.Writer, value AnchorChannelsConfig) {
	FfiConverterSequenceTypePublicKeyINSTANCE.Write(writer, value.TrustedPeersNoReserve)
	FfiConverterUint64INSTANCE.Write(writer, value.PerChannelReserveSats)
}

type FfiDestroyerAnchorChannelsConfig struct{}

func (_ FfiDestroyerAnchorChannelsConfig) Destroy(value AnchorChannelsConfig) {
	value.Destroy()
}

type BackgroundSyncConfig struct {
	OnchainWalletSyncIntervalSecs   uint64
	LightningWalletSyncIntervalSecs uint64
	FeeRateCacheUpdateIntervalSecs  uint64
}

func (r *BackgroundSyncConfig) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.OnchainWalletSyncIntervalSecs)
	FfiDestroyerUint64{}.Destroy(r.LightningWalletSyncIntervalSecs)
	FfiDestroyerUint64{}.Destroy(r.FeeRateCacheUpdateIntervalSecs)
}

type FfiConverterBackgroundSyncConfig struct{}

var FfiConverterBackgroundSyncConfigINSTANCE = FfiConverterBackgroundSyncConfig{}

func (c FfiConverterBackgroundSyncConfig) Lift(rb RustBufferI) BackgroundSyncConfig {
	return LiftFromRustBuffer[BackgroundSyncConfig](c, rb)
}

func (c FfiConverterBackgroundSyncConfig) Read(reader io.Reader) BackgroundSyncConfig {
	return BackgroundSyncConfig{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterBackgroundSyncConfig) Lower(value BackgroundSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[BackgroundSyncConfig](c, value)
}

func (c FfiConverterBackgroundSyncConfig) Write(writer io.Writer, value BackgroundSyncConfig) {
	FfiConverterUint64INSTANCE.Write(writer, value.OnchainWalletSyncIntervalSecs)
	FfiConverterUint64INSTANCE.Write(writer, value.LightningWalletSyncIntervalSecs)
	FfiConverterUint64INSTANCE.Write(writer, value.FeeRateCacheUpdateIntervalSecs)
}

type FfiDestroyerBackgroundSyncConfig struct{}

func (_ FfiDestroyerBackgroundSyncConfig) Destroy(value BackgroundSyncConfig) {
	value.Destroy()
}

type BalanceDetails struct {
	TotalOnchainBalanceSats            uint64
	SpendableOnchainBalanceSats        uint64
	TotalAnchorChannelsReserveSats     uint64
	TotalLightningBalanceSats          uint64
	LightningBalances                  []LightningBalance
	PendingBalancesFromChannelClosures []PendingSweepBalance
}

func (r *BalanceDetails) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.TotalOnchainBalanceSats)
	FfiDestroyerUint64{}.Destroy(r.SpendableOnchainBalanceSats)
	FfiDestroyerUint64{}.Destroy(r.TotalAnchorChannelsReserveSats)
	FfiDestroyerUint64{}.Destroy(r.TotalLightningBalanceSats)
	FfiDestroyerSequenceLightningBalance{}.Destroy(r.LightningBalances)
	FfiDestroyerSequencePendingSweepBalance{}.Destroy(r.PendingBalancesFromChannelClosures)
}

type FfiConverterBalanceDetails struct{}

var FfiConverterBalanceDetailsINSTANCE = FfiConverterBalanceDetails{}

func (c FfiConverterBalanceDetails) Lift(rb RustBufferI) BalanceDetails {
	return LiftFromRustBuffer[BalanceDetails](c, rb)
}

func (c FfiConverterBalanceDetails) Read(reader io.Reader) BalanceDetails {
	return BalanceDetails{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterSequenceLightningBalanceINSTANCE.Read(reader),
		FfiConverterSequencePendingSweepBalanceINSTANCE.Read(reader),
	}
}

func (c FfiConverterBalanceDetails) Lower(value BalanceDetails) C.RustBuffer {
	return LowerIntoRustBuffer[BalanceDetails](c, value)
}

func (c FfiConverterBalanceDetails) Write(writer io.Writer, value BalanceDetails) {
	FfiConverterUint64INSTANCE.Write(writer, value.TotalOnchainBalanceSats)
	FfiConverterUint64INSTANCE.Write(writer, value.SpendableOnchainBalanceSats)
	FfiConverterUint64INSTANCE.Write(writer, value.TotalAnchorChannelsReserveSats)
	FfiConverterUint64INSTANCE.Write(writer, value.TotalLightningBalanceSats)
	FfiConverterSequenceLightningBalanceINSTANCE.Write(writer, value.LightningBalances)
	FfiConverterSequencePendingSweepBalanceINSTANCE.Write(writer, value.PendingBalancesFromChannelClosures)
}

type FfiDestroyerBalanceDetails struct{}

func (_ FfiDestroyerBalanceDetails) Destroy(value BalanceDetails) {
	value.Destroy()
}

type BestBlock struct {
	BlockHash BlockHash
	Height    uint32
}

func (r *BestBlock) Destroy() {
	FfiDestroyerTypeBlockHash{}.Destroy(r.BlockHash)
	FfiDestroyerUint32{}.Destroy(r.Height)
}

type FfiConverterBestBlock struct{}

var FfiConverterBestBlockINSTANCE = FfiConverterBestBlock{}

func (c FfiConverterBestBlock) Lift(rb RustBufferI) BestBlock {
	return LiftFromRustBuffer[BestBlock](c, rb)
}

func (c FfiConverterBestBlock) Read(reader io.Reader) BestBlock {
	return BestBlock{
		FfiConverterTypeBlockHashINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterBestBlock) Lower(value BestBlock) C.RustBuffer {
	return LowerIntoRustBuffer[BestBlock](c, value)
}

func (c FfiConverterBestBlock) Write(writer io.Writer, value BestBlock) {
	FfiConverterTypeBlockHashINSTANCE.Write(writer, value.BlockHash)
	FfiConverterUint32INSTANCE.Write(writer, value.Height)
}

type FfiDestroyerBestBlock struct{}

func (_ FfiDestroyerBestBlock) Destroy(value BestBlock) {
	value.Destroy()
}

type ChannelConfig struct {
	ForwardingFeeProportionalMillionths uint32
	ForwardingFeeBaseMsat               uint32
	CltvExpiryDelta                     uint16
	MaxDustHtlcExposure                 MaxDustHtlcExposure
	ForceCloseAvoidanceMaxFeeSatoshis   uint64
	AcceptUnderpayingHtlcs              bool
}

func (r *ChannelConfig) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.ForwardingFeeProportionalMillionths)
	FfiDestroyerUint32{}.Destroy(r.ForwardingFeeBaseMsat)
	FfiDestroyerUint16{}.Destroy(r.CltvExpiryDelta)
	FfiDestroyerMaxDustHtlcExposure{}.Destroy(r.MaxDustHtlcExposure)
	FfiDestroyerUint64{}.Destroy(r.ForceCloseAvoidanceMaxFeeSatoshis)
	FfiDestroyerBool{}.Destroy(r.AcceptUnderpayingHtlcs)
}

type FfiConverterChannelConfig struct{}

var FfiConverterChannelConfigINSTANCE = FfiConverterChannelConfig{}

func (c FfiConverterChannelConfig) Lift(rb RustBufferI) ChannelConfig {
	return LiftFromRustBuffer[ChannelConfig](c, rb)
}

func (c FfiConverterChannelConfig) Read(reader io.Reader) ChannelConfig {
	return ChannelConfig{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint16INSTANCE.Read(reader),
		FfiConverterMaxDustHtlcExposureINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterChannelConfig) Lower(value ChannelConfig) C.RustBuffer {
	return LowerIntoRustBuffer[ChannelConfig](c, value)
}

func (c FfiConverterChannelConfig) Write(writer io.Writer, value ChannelConfig) {
	FfiConverterUint32INSTANCE.Write(writer, value.ForwardingFeeProportionalMillionths)
	FfiConverterUint32INSTANCE.Write(writer, value.ForwardingFeeBaseMsat)
	FfiConverterUint16INSTANCE.Write(writer, value.CltvExpiryDelta)
	FfiConverterMaxDustHtlcExposureINSTANCE.Write(writer, value.MaxDustHtlcExposure)
	FfiConverterUint64INSTANCE.Write(writer, value.ForceCloseAvoidanceMaxFeeSatoshis)
	FfiConverterBoolINSTANCE.Write(writer, value.AcceptUnderpayingHtlcs)
}

type FfiDestroyerChannelConfig struct{}

func (_ FfiDestroyerChannelConfig) Destroy(value ChannelConfig) {
	value.Destroy()
}

type ChannelDetails struct {
	ChannelId                                           ChannelId
	CounterpartyNodeId                                  PublicKey
	FundingTxo                                          *OutPoint
	ShortChannelId                                      *uint64
	OutboundScidAlias                                   *uint64
	InboundScidAlias                                    *uint64
	ChannelValueSats                                    uint64
	UnspendablePunishmentReserve                        *uint64
	UserChannelId                                       UserChannelId
	FeerateSatPer1000Weight                             uint32
	OutboundCapacityMsat                                uint64
	InboundCapacityMsat                                 uint64
	ConfirmationsRequired                               *uint32
	Confirmations                                       *uint32
	IsOutbound                                          bool
	IsChannelReady                                      bool
	IsUsable                                            bool
	IsAnnounced                                         bool
	CltvExpiryDelta                                     *uint16
	CounterpartyUnspendablePunishmentReserve            uint64
	CounterpartyOutboundHtlcMinimumMsat                 *uint64
	CounterpartyOutboundHtlcMaximumMsat                 *uint64
	CounterpartyForwardingInfoFeeBaseMsat               *uint32
	CounterpartyForwardingInfoFeeProportionalMillionths *uint32
	CounterpartyForwardingInfoCltvExpiryDelta           *uint16
	NextOutboundHtlcLimitMsat                           uint64
	NextOutboundHtlcMinimumMsat                         uint64
	ForceCloseSpendDelay                                *uint16
	InboundHtlcMinimumMsat                              uint64
	InboundHtlcMaximumMsat                              *uint64
	Config                                              ChannelConfig
}

func (r *ChannelDetails) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(r.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(r.CounterpartyNodeId)
	FfiDestroyerOptionalOutPoint{}.Destroy(r.FundingTxo)
	FfiDestroyerOptionalUint64{}.Destroy(r.ShortChannelId)
	FfiDestroyerOptionalUint64{}.Destroy(r.OutboundScidAlias)
	FfiDestroyerOptionalUint64{}.Destroy(r.InboundScidAlias)
	FfiDestroyerUint64{}.Destroy(r.ChannelValueSats)
	FfiDestroyerOptionalUint64{}.Destroy(r.UnspendablePunishmentReserve)
	FfiDestroyerTypeUserChannelId{}.Destroy(r.UserChannelId)
	FfiDestroyerUint32{}.Destroy(r.FeerateSatPer1000Weight)
	FfiDestroyerUint64{}.Destroy(r.OutboundCapacityMsat)
	FfiDestroyerUint64{}.Destroy(r.InboundCapacityMsat)
	FfiDestroyerOptionalUint32{}.Destroy(r.ConfirmationsRequired)
	FfiDestroyerOptionalUint32{}.Destroy(r.Confirmations)
	FfiDestroyerBool{}.Destroy(r.IsOutbound)
	FfiDestroyerBool{}.Destroy(r.IsChannelReady)
	FfiDestroyerBool{}.Destroy(r.IsUsable)
	FfiDestroyerBool{}.Destroy(r.IsAnnounced)
	FfiDestroyerOptionalUint16{}.Destroy(r.CltvExpiryDelta)
	FfiDestroyerUint64{}.Destroy(r.CounterpartyUnspendablePunishmentReserve)
	FfiDestroyerOptionalUint64{}.Destroy(r.CounterpartyOutboundHtlcMinimumMsat)
	FfiDestroyerOptionalUint64{}.Destroy(r.CounterpartyOutboundHtlcMaximumMsat)
	FfiDestroyerOptionalUint32{}.Destroy(r.CounterpartyForwardingInfoFeeBaseMsat)
	FfiDestroyerOptionalUint32{}.Destroy(r.CounterpartyForwardingInfoFeeProportionalMillionths)
	FfiDestroyerOptionalUint16{}.Destroy(r.CounterpartyForwardingInfoCltvExpiryDelta)
	FfiDestroyerUint64{}.Destroy(r.NextOutboundHtlcLimitMsat)
	FfiDestroyerUint64{}.Destroy(r.NextOutboundHtlcMinimumMsat)
	FfiDestroyerOptionalUint16{}.Destroy(r.ForceCloseSpendDelay)
	FfiDestroyerUint64{}.Destroy(r.InboundHtlcMinimumMsat)
	FfiDestroyerOptionalUint64{}.Destroy(r.InboundHtlcMaximumMsat)
	FfiDestroyerChannelConfig{}.Destroy(r.Config)
}

type FfiConverterChannelDetails struct{}

var FfiConverterChannelDetailsINSTANCE = FfiConverterChannelDetails{}

func (c FfiConverterChannelDetails) Lift(rb RustBufferI) ChannelDetails {
	return LiftFromRustBuffer[ChannelDetails](c, rb)
}

func (c FfiConverterChannelDetails) Read(reader io.Reader) ChannelDetails {
	return ChannelDetails{
		FfiConverterTypeChannelIdINSTANCE.Read(reader),
		FfiConverterTypePublicKeyINSTANCE.Read(reader),
		FfiConverterOptionalOutPointINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterOptionalUint16INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
		FfiConverterOptionalUint16INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint16INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterChannelConfigINSTANCE.Read(reader),
	}
}

func (c FfiConverterChannelDetails) Lower(value ChannelDetails) C.RustBuffer {
	return LowerIntoRustBuffer[ChannelDetails](c, value)
}

func (c FfiConverterChannelDetails) Write(writer io.Writer, value ChannelDetails) {
	FfiConverterTypeChannelIdINSTANCE.Write(writer, value.ChannelId)
	FfiConverterTypePublicKeyINSTANCE.Write(writer, value.CounterpartyNodeId)
	FfiConverterOptionalOutPointINSTANCE.Write(writer, value.FundingTxo)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.ShortChannelId)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.OutboundScidAlias)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.InboundScidAlias)
	FfiConverterUint64INSTANCE.Write(writer, value.ChannelValueSats)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.UnspendablePunishmentReserve)
	FfiConverterTypeUserChannelIdINSTANCE.Write(writer, value.UserChannelId)
	FfiConverterUint32INSTANCE.Write(writer, value.FeerateSatPer1000Weight)
	FfiConverterUint64INSTANCE.Write(writer, value.OutboundCapacityMsat)
	FfiConverterUint64INSTANCE.Write(writer, value.InboundCapacityMsat)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.ConfirmationsRequired)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.Confirmations)
	FfiConverterBoolINSTANCE.Write(writer, value.IsOutbound)
	FfiConverterBoolINSTANCE.Write(writer, value.IsChannelReady)
	FfiConverterBoolINSTANCE.Write(writer, value.IsUsable)
	FfiConverterBoolINSTANCE.Write(writer, value.IsAnnounced)
	FfiConverterOptionalUint16INSTANCE.Write(writer, value.CltvExpiryDelta)
	FfiConverterUint64INSTANCE.Write(writer, value.CounterpartyUnspendablePunishmentReserve)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.CounterpartyOutboundHtlcMinimumMsat)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.CounterpartyOutboundHtlcMaximumMsat)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.CounterpartyForwardingInfoFeeBaseMsat)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.CounterpartyForwardingInfoFeeProportionalMillionths)
	FfiConverterOptionalUint16INSTANCE.Write(writer, value.CounterpartyForwardingInfoCltvExpiryDelta)
	FfiConverterUint64INSTANCE.Write(writer, value.NextOutboundHtlcLimitMsat)
	FfiConverterUint64INSTANCE.Write(writer, value.NextOutboundHtlcMinimumMsat)
	FfiConverterOptionalUint16INSTANCE.Write(writer, value.ForceCloseSpendDelay)
	FfiConverterUint64INSTANCE.Write(writer, value.InboundHtlcMinimumMsat)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.InboundHtlcMaximumMsat)
	FfiConverterChannelConfigINSTANCE.Write(writer, value.Config)
}

type FfiDestroyerChannelDetails struct{}

func (_ FfiDestroyerChannelDetails) Destroy(value ChannelDetails) {
	value.Destroy()
}

type ChannelInfo struct {
	NodeOne      NodeId
	OneToTwo     *ChannelUpdateInfo
	NodeTwo      NodeId
	TwoToOne     *ChannelUpdateInfo
	CapacitySats *uint64
}

func (r *ChannelInfo) Destroy() {
	FfiDestroyerTypeNodeId{}.Destroy(r.NodeOne)
	FfiDestroyerOptionalChannelUpdateInfo{}.Destroy(r.OneToTwo)
	FfiDestroyerTypeNodeId{}.Destroy(r.NodeTwo)
	FfiDestroyerOptionalChannelUpdateInfo{}.Destroy(r.TwoToOne)
	FfiDestroyerOptionalUint64{}.Destroy(r.CapacitySats)
}

type FfiConverterChannelInfo struct{}

var FfiConverterChannelInfoINSTANCE = FfiConverterChannelInfo{}

func (c FfiConverterChannelInfo) Lift(rb RustBufferI) ChannelInfo {
	return LiftFromRustBuffer[ChannelInfo](c, rb)
}

func (c FfiConverterChannelInfo) Read(reader io.Reader) ChannelInfo {
	return ChannelInfo{
		FfiConverterTypeNodeIdINSTANCE.Read(reader),
		FfiConverterOptionalChannelUpdateInfoINSTANCE.Read(reader),
		FfiConverterTypeNodeIdINSTANCE.Read(reader),
		FfiConverterOptionalChannelUpdateInfoINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterChannelInfo) Lower(value ChannelInfo) C.RustBuffer {
	return LowerIntoRustBuffer[ChannelInfo](c, value)
}

func (c FfiConverterChannelInfo) Write(writer io.Writer, value ChannelInfo) {
	FfiConverterTypeNodeIdINSTANCE.Write(writer, value.NodeOne)
	FfiConverterOptionalChannelUpdateInfoINSTANCE.Write(writer, value.OneToTwo)
	FfiConverterTypeNodeIdINSTANCE.Write(writer, value.NodeTwo)
	FfiConverterOptionalChannelUpdateInfoINSTANCE.Write(writer, value.TwoToOne)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.CapacitySats)
}

type FfiDestroyerChannelInfo struct{}

func (_ FfiDestroyerChannelInfo) Destroy(value ChannelInfo) {
	value.Destroy()
}

type ChannelMonitorSizeInfo struct {
	ChannelId ChannelId
	SizeBytes uint64
}

func (r *ChannelMonitorSizeInfo) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(r.ChannelId)
	FfiDestroyerUint64{}.Destroy(r.SizeBytes)
}

type FfiConverterChannelMonitorSizeInfo struct{}

var FfiConverterChannelMonitorSizeInfoINSTANCE = FfiConverterChannelMonitorSizeInfo{}

func (c FfiConverterChannelMonitorSizeInfo) Lift(rb RustBufferI) ChannelMonitorSizeInfo {
	return LiftFromRustBuffer[ChannelMonitorSizeInfo](c, rb)
}

func (c FfiConverterChannelMonitorSizeInfo) Read(reader io.Reader) ChannelMonitorSizeInfo {
	return ChannelMonitorSizeInfo{
		FfiConverterTypeChannelIdINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterChannelMonitorSizeInfo) Lower(value ChannelMonitorSizeInfo) C.RustBuffer {
	return LowerIntoRustBuffer[ChannelMonitorSizeInfo](c, value)
}

func (c FfiConverterChannelMonitorSizeInfo) Write(writer io.Writer, value ChannelMonitorSizeInfo) {
	FfiConverterTypeChannelIdINSTANCE.Write(writer, value.ChannelId)
	FfiConverterUint64INSTANCE.Write(writer, value.SizeBytes)
}

type FfiDestroyerChannelMonitorSizeInfo struct{}

func (_ FfiDestroyerChannelMonitorSizeInfo) Destroy(value ChannelMonitorSizeInfo) {
	value.Destroy()
}

type ChannelUpdateInfo struct {
	LastUpdate      uint32
	Enabled         bool
	CltvExpiryDelta uint16
	HtlcMinimumMsat uint64
	HtlcMaximumMsat uint64
	Fees            RoutingFees
}

func (r *ChannelUpdateInfo) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.LastUpdate)
	FfiDestroyerBool{}.Destroy(r.Enabled)
	FfiDestroyerUint16{}.Destroy(r.CltvExpiryDelta)
	FfiDestroyerUint64{}.Destroy(r.HtlcMinimumMsat)
	FfiDestroyerUint64{}.Destroy(r.HtlcMaximumMsat)
	FfiDestroyerRoutingFees{}.Destroy(r.Fees)
}

type FfiConverterChannelUpdateInfo struct{}

var FfiConverterChannelUpdateInfoINSTANCE = FfiConverterChannelUpdateInfo{}

func (c FfiConverterChannelUpdateInfo) Lift(rb RustBufferI) ChannelUpdateInfo {
	return LiftFromRustBuffer[ChannelUpdateInfo](c, rb)
}

func (c FfiConverterChannelUpdateInfo) Read(reader io.Reader) ChannelUpdateInfo {
	return ChannelUpdateInfo{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterUint16INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterRoutingFeesINSTANCE.Read(reader),
	}
}

func (c FfiConverterChannelUpdateInfo) Lower(value ChannelUpdateInfo) C.RustBuffer {
	return LowerIntoRustBuffer[ChannelUpdateInfo](c, value)
}

func (c FfiConverterChannelUpdateInfo) Write(writer io.Writer, value ChannelUpdateInfo) {
	FfiConverterUint32INSTANCE.Write(writer, value.LastUpdate)
	FfiConverterBoolINSTANCE.Write(writer, value.Enabled)
	FfiConverterUint16INSTANCE.Write(writer, value.CltvExpiryDelta)
	FfiConverterUint64INSTANCE.Write(writer, value.HtlcMinimumMsat)
	FfiConverterUint64INSTANCE.Write(writer, value.HtlcMaximumMsat)
	FfiConverterRoutingFeesINSTANCE.Write(writer, value.Fees)
}

type FfiDestroyerChannelUpdateInfo struct{}

func (_ FfiDestroyerChannelUpdateInfo) Destroy(value ChannelUpdateInfo) {
	value.Destroy()
}

type Config struct {
	StorageDirPath                  string
	Network                         Network
	ListeningAddresses              *[]SocketAddress
	AnnouncementAddresses           *[]SocketAddress
	NodeAlias                       *NodeAlias
	TrustedPeers0conf               []PublicKey
	ProbingLiquidityLimitMultiplier uint64
	AnchorChannelsConfig            *AnchorChannelsConfig
	TransientNetworkGraph           bool
	RouteParameters                 *RouteParametersConfig
}

func (r *Config) Destroy() {
	FfiDestroyerString{}.Destroy(r.StorageDirPath)
	FfiDestroyerNetwork{}.Destroy(r.Network)
	FfiDestroyerOptionalSequenceTypeSocketAddress{}.Destroy(r.ListeningAddresses)
	FfiDestroyerOptionalSequenceTypeSocketAddress{}.Destroy(r.AnnouncementAddresses)
	FfiDestroyerOptionalTypeNodeAlias{}.Destroy(r.NodeAlias)
	FfiDestroyerSequenceTypePublicKey{}.Destroy(r.TrustedPeers0conf)
	FfiDestroyerUint64{}.Destroy(r.ProbingLiquidityLimitMultiplier)
	FfiDestroyerOptionalAnchorChannelsConfig{}.Destroy(r.AnchorChannelsConfig)
	FfiDestroyerBool{}.Destroy(r.TransientNetworkGraph)
	FfiDestroyerOptionalRouteParametersConfig{}.Destroy(r.RouteParameters)
}

type FfiConverterConfig struct{}

var FfiConverterConfigINSTANCE = FfiConverterConfig{}

func (c FfiConverterConfig) Lift(rb RustBufferI) Config {
	return LiftFromRustBuffer[Config](c, rb)
}

func (c FfiConverterConfig) Read(reader io.Reader) Config {
	return Config{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterNetworkINSTANCE.Read(reader),
		FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Read(reader),
		FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Read(reader),
		FfiConverterOptionalTypeNodeAliasINSTANCE.Read(reader),
		FfiConverterSequenceTypePublicKeyINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterOptionalAnchorChannelsConfigINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterOptionalRouteParametersConfigINSTANCE.Read(reader),
	}
}

func (c FfiConverterConfig) Lower(value Config) C.RustBuffer {
	return LowerIntoRustBuffer[Config](c, value)
}

func (c FfiConverterConfig) Write(writer io.Writer, value Config) {
	FfiConverterStringINSTANCE.Write(writer, value.StorageDirPath)
	FfiConverterNetworkINSTANCE.Write(writer, value.Network)
	FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Write(writer, value.ListeningAddresses)
	FfiConverterOptionalSequenceTypeSocketAddressINSTANCE.Write(writer, value.AnnouncementAddresses)
	FfiConverterOptionalTypeNodeAliasINSTANCE.Write(writer, value.NodeAlias)
	FfiConverterSequenceTypePublicKeyINSTANCE.Write(writer, value.TrustedPeers0conf)
	FfiConverterUint64INSTANCE.Write(writer, value.ProbingLiquidityLimitMultiplier)
	FfiConverterOptionalAnchorChannelsConfigINSTANCE.Write(writer, value.AnchorChannelsConfig)
	FfiConverterBoolINSTANCE.Write(writer, value.TransientNetworkGraph)
	FfiConverterOptionalRouteParametersConfigINSTANCE.Write(writer, value.RouteParameters)
}

type FfiDestroyerConfig struct{}

func (_ FfiDestroyerConfig) Destroy(value Config) {
	value.Destroy()
}

type CustomTlvRecord struct {
	TypeNum uint64
	Value   []uint8
}

func (r *CustomTlvRecord) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.TypeNum)
	FfiDestroyerSequenceUint8{}.Destroy(r.Value)
}

type FfiConverterCustomTlvRecord struct{}

var FfiConverterCustomTlvRecordINSTANCE = FfiConverterCustomTlvRecord{}

func (c FfiConverterCustomTlvRecord) Lift(rb RustBufferI) CustomTlvRecord {
	return LiftFromRustBuffer[CustomTlvRecord](c, rb)
}

func (c FfiConverterCustomTlvRecord) Read(reader io.Reader) CustomTlvRecord {
	return CustomTlvRecord{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterSequenceUint8INSTANCE.Read(reader),
	}
}

func (c FfiConverterCustomTlvRecord) Lower(value CustomTlvRecord) C.RustBuffer {
	return LowerIntoRustBuffer[CustomTlvRecord](c, value)
}

func (c FfiConverterCustomTlvRecord) Write(writer io.Writer, value CustomTlvRecord) {
	FfiConverterUint64INSTANCE.Write(writer, value.TypeNum)
	FfiConverterSequenceUint8INSTANCE.Write(writer, value.Value)
}

type FfiDestroyerCustomTlvRecord struct{}

func (_ FfiDestroyerCustomTlvRecord) Destroy(value CustomTlvRecord) {
	value.Destroy()
}

type ElectrumSyncConfig struct {
	BackgroundSyncConfig *BackgroundSyncConfig
}

func (r *ElectrumSyncConfig) Destroy() {
	FfiDestroyerOptionalBackgroundSyncConfig{}.Destroy(r.BackgroundSyncConfig)
}

type FfiConverterElectrumSyncConfig struct{}

var FfiConverterElectrumSyncConfigINSTANCE = FfiConverterElectrumSyncConfig{}

func (c FfiConverterElectrumSyncConfig) Lift(rb RustBufferI) ElectrumSyncConfig {
	return LiftFromRustBuffer[ElectrumSyncConfig](c, rb)
}

func (c FfiConverterElectrumSyncConfig) Read(reader io.Reader) ElectrumSyncConfig {
	return ElectrumSyncConfig{
		FfiConverterOptionalBackgroundSyncConfigINSTANCE.Read(reader),
	}
}

func (c FfiConverterElectrumSyncConfig) Lower(value ElectrumSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[ElectrumSyncConfig](c, value)
}

func (c FfiConverterElectrumSyncConfig) Write(writer io.Writer, value ElectrumSyncConfig) {
	FfiConverterOptionalBackgroundSyncConfigINSTANCE.Write(writer, value.BackgroundSyncConfig)
}

type FfiDestroyerElectrumSyncConfig struct{}

func (_ FfiDestroyerElectrumSyncConfig) Destroy(value ElectrumSyncConfig) {
	value.Destroy()
}

type EsploraSyncConfig struct {
	BackgroundSyncConfig *BackgroundSyncConfig
}

func (r *EsploraSyncConfig) Destroy() {
	FfiDestroyerOptionalBackgroundSyncConfig{}.Destroy(r.BackgroundSyncConfig)
}

type FfiConverterEsploraSyncConfig struct{}

var FfiConverterEsploraSyncConfigINSTANCE = FfiConverterEsploraSyncConfig{}

func (c FfiConverterEsploraSyncConfig) Lift(rb RustBufferI) EsploraSyncConfig {
	return LiftFromRustBuffer[EsploraSyncConfig](c, rb)
}

func (c FfiConverterEsploraSyncConfig) Read(reader io.Reader) EsploraSyncConfig {
	return EsploraSyncConfig{
		FfiConverterOptionalBackgroundSyncConfigINSTANCE.Read(reader),
	}
}

func (c FfiConverterEsploraSyncConfig) Lower(value EsploraSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[EsploraSyncConfig](c, value)
}

func (c FfiConverterEsploraSyncConfig) Write(writer io.Writer, value EsploraSyncConfig) {
	FfiConverterOptionalBackgroundSyncConfigINSTANCE.Write(writer, value.BackgroundSyncConfig)
}

type FfiDestroyerEsploraSyncConfig struct{}

func (_ FfiDestroyerEsploraSyncConfig) Destroy(value EsploraSyncConfig) {
	value.Destroy()
}

type KeyValue struct {
	Key   string
	Value []uint8
}

func (r *KeyValue) Destroy() {
	FfiDestroyerString{}.Destroy(r.Key)
	FfiDestroyerSequenceUint8{}.Destroy(r.Value)
}

type FfiConverterKeyValue struct{}

var FfiConverterKeyValueINSTANCE = FfiConverterKeyValue{}

func (c FfiConverterKeyValue) Lift(rb RustBufferI) KeyValue {
	return LiftFromRustBuffer[KeyValue](c, rb)
}

func (c FfiConverterKeyValue) Read(reader io.Reader) KeyValue {
	return KeyValue{
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterSequenceUint8INSTANCE.Read(reader),
	}
}

func (c FfiConverterKeyValue) Lower(value KeyValue) C.RustBuffer {
	return LowerIntoRustBuffer[KeyValue](c, value)
}

func (c FfiConverterKeyValue) Write(writer io.Writer, value KeyValue) {
	FfiConverterStringINSTANCE.Write(writer, value.Key)
	FfiConverterSequenceUint8INSTANCE.Write(writer, value.Value)
}

type FfiDestroyerKeyValue struct{}

func (_ FfiDestroyerKeyValue) Destroy(value KeyValue) {
	value.Destroy()
}

type LspFeeLimits struct {
	MaxTotalOpeningFeeMsat           *uint64
	MaxProportionalOpeningFeePpmMsat *uint64
}

func (r *LspFeeLimits) Destroy() {
	FfiDestroyerOptionalUint64{}.Destroy(r.MaxTotalOpeningFeeMsat)
	FfiDestroyerOptionalUint64{}.Destroy(r.MaxProportionalOpeningFeePpmMsat)
}

type FfiConverterLspFeeLimits struct{}

var FfiConverterLspFeeLimitsINSTANCE = FfiConverterLspFeeLimits{}

func (c FfiConverterLspFeeLimits) Lift(rb RustBufferI) LspFeeLimits {
	return LiftFromRustBuffer[LspFeeLimits](c, rb)
}

func (c FfiConverterLspFeeLimits) Read(reader io.Reader) LspFeeLimits {
	return LspFeeLimits{
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterLspFeeLimits) Lower(value LspFeeLimits) C.RustBuffer {
	return LowerIntoRustBuffer[LspFeeLimits](c, value)
}

func (c FfiConverterLspFeeLimits) Write(writer io.Writer, value LspFeeLimits) {
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.MaxTotalOpeningFeeMsat)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.MaxProportionalOpeningFeePpmMsat)
}

type FfiDestroyerLspFeeLimits struct{}

func (_ FfiDestroyerLspFeeLimits) Destroy(value LspFeeLimits) {
	value.Destroy()
}

type Lsps1Bolt11PaymentInfo struct {
	State         Lsps1PaymentState
	ExpiresAt     LSPSDateTime
	FeeTotalSat   uint64
	OrderTotalSat uint64
	Invoice       *Bolt11Invoice
}

func (r *Lsps1Bolt11PaymentInfo) Destroy() {
	FfiDestroyerLsps1PaymentState{}.Destroy(r.State)
	FfiDestroyerTypeLSPSDateTime{}.Destroy(r.ExpiresAt)
	FfiDestroyerUint64{}.Destroy(r.FeeTotalSat)
	FfiDestroyerUint64{}.Destroy(r.OrderTotalSat)
	FfiDestroyerBolt11Invoice{}.Destroy(r.Invoice)
}

type FfiConverterLsps1Bolt11PaymentInfo struct{}

var FfiConverterLsps1Bolt11PaymentInfoINSTANCE = FfiConverterLsps1Bolt11PaymentInfo{}

func (c FfiConverterLsps1Bolt11PaymentInfo) Lift(rb RustBufferI) Lsps1Bolt11PaymentInfo {
	return LiftFromRustBuffer[Lsps1Bolt11PaymentInfo](c, rb)
}

func (c FfiConverterLsps1Bolt11PaymentInfo) Read(reader io.Reader) Lsps1Bolt11PaymentInfo {
	return Lsps1Bolt11PaymentInfo{
		FfiConverterLsps1PaymentStateINSTANCE.Read(reader),
		FfiConverterTypeLSPSDateTimeINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterBolt11InvoiceINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1Bolt11PaymentInfo) Lower(value Lsps1Bolt11PaymentInfo) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1Bolt11PaymentInfo](c, value)
}

func (c FfiConverterLsps1Bolt11PaymentInfo) Write(writer io.Writer, value Lsps1Bolt11PaymentInfo) {
	FfiConverterLsps1PaymentStateINSTANCE.Write(writer, value.State)
	FfiConverterTypeLSPSDateTimeINSTANCE.Write(writer, value.ExpiresAt)
	FfiConverterUint64INSTANCE.Write(writer, value.FeeTotalSat)
	FfiConverterUint64INSTANCE.Write(writer, value.OrderTotalSat)
	FfiConverterBolt11InvoiceINSTANCE.Write(writer, value.Invoice)
}

type FfiDestroyerLsps1Bolt11PaymentInfo struct{}

func (_ FfiDestroyerLsps1Bolt11PaymentInfo) Destroy(value Lsps1Bolt11PaymentInfo) {
	value.Destroy()
}

type Lsps1ChannelInfo struct {
	FundedAt        LSPSDateTime
	FundingOutpoint OutPoint
	ExpiresAt       LSPSDateTime
}

func (r *Lsps1ChannelInfo) Destroy() {
	FfiDestroyerTypeLSPSDateTime{}.Destroy(r.FundedAt)
	FfiDestroyerOutPoint{}.Destroy(r.FundingOutpoint)
	FfiDestroyerTypeLSPSDateTime{}.Destroy(r.ExpiresAt)
}

type FfiConverterLsps1ChannelInfo struct{}

var FfiConverterLsps1ChannelInfoINSTANCE = FfiConverterLsps1ChannelInfo{}

func (c FfiConverterLsps1ChannelInfo) Lift(rb RustBufferI) Lsps1ChannelInfo {
	return LiftFromRustBuffer[Lsps1ChannelInfo](c, rb)
}

func (c FfiConverterLsps1ChannelInfo) Read(reader io.Reader) Lsps1ChannelInfo {
	return Lsps1ChannelInfo{
		FfiConverterTypeLSPSDateTimeINSTANCE.Read(reader),
		FfiConverterOutPointINSTANCE.Read(reader),
		FfiConverterTypeLSPSDateTimeINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1ChannelInfo) Lower(value Lsps1ChannelInfo) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1ChannelInfo](c, value)
}

func (c FfiConverterLsps1ChannelInfo) Write(writer io.Writer, value Lsps1ChannelInfo) {
	FfiConverterTypeLSPSDateTimeINSTANCE.Write(writer, value.FundedAt)
	FfiConverterOutPointINSTANCE.Write(writer, value.FundingOutpoint)
	FfiConverterTypeLSPSDateTimeINSTANCE.Write(writer, value.ExpiresAt)
}

type FfiDestroyerLsps1ChannelInfo struct{}

func (_ FfiDestroyerLsps1ChannelInfo) Destroy(value Lsps1ChannelInfo) {
	value.Destroy()
}

type Lsps1OnchainPaymentInfo struct {
	State                          Lsps1PaymentState
	ExpiresAt                      LSPSDateTime
	FeeTotalSat                    uint64
	OrderTotalSat                  uint64
	Address                        Address
	MinOnchainPaymentConfirmations *uint16
	MinFeeFor0conf                 *FeeRate
	RefundOnchainAddress           *Address
}

func (r *Lsps1OnchainPaymentInfo) Destroy() {
	FfiDestroyerLsps1PaymentState{}.Destroy(r.State)
	FfiDestroyerTypeLSPSDateTime{}.Destroy(r.ExpiresAt)
	FfiDestroyerUint64{}.Destroy(r.FeeTotalSat)
	FfiDestroyerUint64{}.Destroy(r.OrderTotalSat)
	FfiDestroyerTypeAddress{}.Destroy(r.Address)
	FfiDestroyerOptionalUint16{}.Destroy(r.MinOnchainPaymentConfirmations)
	FfiDestroyerFeeRate{}.Destroy(r.MinFeeFor0conf)
	FfiDestroyerOptionalTypeAddress{}.Destroy(r.RefundOnchainAddress)
}

type FfiConverterLsps1OnchainPaymentInfo struct{}

var FfiConverterLsps1OnchainPaymentInfoINSTANCE = FfiConverterLsps1OnchainPaymentInfo{}

func (c FfiConverterLsps1OnchainPaymentInfo) Lift(rb RustBufferI) Lsps1OnchainPaymentInfo {
	return LiftFromRustBuffer[Lsps1OnchainPaymentInfo](c, rb)
}

func (c FfiConverterLsps1OnchainPaymentInfo) Read(reader io.Reader) Lsps1OnchainPaymentInfo {
	return Lsps1OnchainPaymentInfo{
		FfiConverterLsps1PaymentStateINSTANCE.Read(reader),
		FfiConverterTypeLSPSDateTimeINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterTypeAddressINSTANCE.Read(reader),
		FfiConverterOptionalUint16INSTANCE.Read(reader),
		FfiConverterFeeRateINSTANCE.Read(reader),
		FfiConverterOptionalTypeAddressINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1OnchainPaymentInfo) Lower(value Lsps1OnchainPaymentInfo) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1OnchainPaymentInfo](c, value)
}

func (c FfiConverterLsps1OnchainPaymentInfo) Write(writer io.Writer, value Lsps1OnchainPaymentInfo) {
	FfiConverterLsps1PaymentStateINSTANCE.Write(writer, value.State)
	FfiConverterTypeLSPSDateTimeINSTANCE.Write(writer, value.ExpiresAt)
	FfiConverterUint64INSTANCE.Write(writer, value.FeeTotalSat)
	FfiConverterUint64INSTANCE.Write(writer, value.OrderTotalSat)
	FfiConverterTypeAddressINSTANCE.Write(writer, value.Address)
	FfiConverterOptionalUint16INSTANCE.Write(writer, value.MinOnchainPaymentConfirmations)
	FfiConverterFeeRateINSTANCE.Write(writer, value.MinFeeFor0conf)
	FfiConverterOptionalTypeAddressINSTANCE.Write(writer, value.RefundOnchainAddress)
}

type FfiDestroyerLsps1OnchainPaymentInfo struct{}

func (_ FfiDestroyerLsps1OnchainPaymentInfo) Destroy(value Lsps1OnchainPaymentInfo) {
	value.Destroy()
}

type Lsps1OrderParams struct {
	LspBalanceSat                uint64
	ClientBalanceSat             uint64
	RequiredChannelConfirmations uint16
	FundingConfirmsWithinBlocks  uint16
	ChannelExpiryBlocks          uint32
	Token                        *string
	AnnounceChannel              bool
}

func (r *Lsps1OrderParams) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.LspBalanceSat)
	FfiDestroyerUint64{}.Destroy(r.ClientBalanceSat)
	FfiDestroyerUint16{}.Destroy(r.RequiredChannelConfirmations)
	FfiDestroyerUint16{}.Destroy(r.FundingConfirmsWithinBlocks)
	FfiDestroyerUint32{}.Destroy(r.ChannelExpiryBlocks)
	FfiDestroyerOptionalString{}.Destroy(r.Token)
	FfiDestroyerBool{}.Destroy(r.AnnounceChannel)
}

type FfiConverterLsps1OrderParams struct{}

var FfiConverterLsps1OrderParamsINSTANCE = FfiConverterLsps1OrderParams{}

func (c FfiConverterLsps1OrderParams) Lift(rb RustBufferI) Lsps1OrderParams {
	return LiftFromRustBuffer[Lsps1OrderParams](c, rb)
}

func (c FfiConverterLsps1OrderParams) Read(reader io.Reader) Lsps1OrderParams {
	return Lsps1OrderParams{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint16INSTANCE.Read(reader),
		FfiConverterUint16INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1OrderParams) Lower(value Lsps1OrderParams) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1OrderParams](c, value)
}

func (c FfiConverterLsps1OrderParams) Write(writer io.Writer, value Lsps1OrderParams) {
	FfiConverterUint64INSTANCE.Write(writer, value.LspBalanceSat)
	FfiConverterUint64INSTANCE.Write(writer, value.ClientBalanceSat)
	FfiConverterUint16INSTANCE.Write(writer, value.RequiredChannelConfirmations)
	FfiConverterUint16INSTANCE.Write(writer, value.FundingConfirmsWithinBlocks)
	FfiConverterUint32INSTANCE.Write(writer, value.ChannelExpiryBlocks)
	FfiConverterOptionalStringINSTANCE.Write(writer, value.Token)
	FfiConverterBoolINSTANCE.Write(writer, value.AnnounceChannel)
}

type FfiDestroyerLsps1OrderParams struct{}

func (_ FfiDestroyerLsps1OrderParams) Destroy(value Lsps1OrderParams) {
	value.Destroy()
}

type Lsps1OrderStatus struct {
	OrderId        LSPS1OrderId
	OrderParams    Lsps1OrderParams
	PaymentOptions Lsps1PaymentInfo
	ChannelState   *Lsps1ChannelInfo
}

func (r *Lsps1OrderStatus) Destroy() {
	FfiDestroyerTypeLSPS1OrderId{}.Destroy(r.OrderId)
	FfiDestroyerLsps1OrderParams{}.Destroy(r.OrderParams)
	FfiDestroyerLsps1PaymentInfo{}.Destroy(r.PaymentOptions)
	FfiDestroyerOptionalLsps1ChannelInfo{}.Destroy(r.ChannelState)
}

type FfiConverterLsps1OrderStatus struct{}

var FfiConverterLsps1OrderStatusINSTANCE = FfiConverterLsps1OrderStatus{}

func (c FfiConverterLsps1OrderStatus) Lift(rb RustBufferI) Lsps1OrderStatus {
	return LiftFromRustBuffer[Lsps1OrderStatus](c, rb)
}

func (c FfiConverterLsps1OrderStatus) Read(reader io.Reader) Lsps1OrderStatus {
	return Lsps1OrderStatus{
		FfiConverterTypeLSPS1OrderIdINSTANCE.Read(reader),
		FfiConverterLsps1OrderParamsINSTANCE.Read(reader),
		FfiConverterLsps1PaymentInfoINSTANCE.Read(reader),
		FfiConverterOptionalLsps1ChannelInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1OrderStatus) Lower(value Lsps1OrderStatus) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1OrderStatus](c, value)
}

func (c FfiConverterLsps1OrderStatus) Write(writer io.Writer, value Lsps1OrderStatus) {
	FfiConverterTypeLSPS1OrderIdINSTANCE.Write(writer, value.OrderId)
	FfiConverterLsps1OrderParamsINSTANCE.Write(writer, value.OrderParams)
	FfiConverterLsps1PaymentInfoINSTANCE.Write(writer, value.PaymentOptions)
	FfiConverterOptionalLsps1ChannelInfoINSTANCE.Write(writer, value.ChannelState)
}

type FfiDestroyerLsps1OrderStatus struct{}

func (_ FfiDestroyerLsps1OrderStatus) Destroy(value Lsps1OrderStatus) {
	value.Destroy()
}

type Lsps1PaymentInfo struct {
	Bolt11  *Lsps1Bolt11PaymentInfo
	Onchain *Lsps1OnchainPaymentInfo
}

func (r *Lsps1PaymentInfo) Destroy() {
	FfiDestroyerOptionalLsps1Bolt11PaymentInfo{}.Destroy(r.Bolt11)
	FfiDestroyerOptionalLsps1OnchainPaymentInfo{}.Destroy(r.Onchain)
}

type FfiConverterLsps1PaymentInfo struct{}

var FfiConverterLsps1PaymentInfoINSTANCE = FfiConverterLsps1PaymentInfo{}

func (c FfiConverterLsps1PaymentInfo) Lift(rb RustBufferI) Lsps1PaymentInfo {
	return LiftFromRustBuffer[Lsps1PaymentInfo](c, rb)
}

func (c FfiConverterLsps1PaymentInfo) Read(reader io.Reader) Lsps1PaymentInfo {
	return Lsps1PaymentInfo{
		FfiConverterOptionalLsps1Bolt11PaymentInfoINSTANCE.Read(reader),
		FfiConverterOptionalLsps1OnchainPaymentInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps1PaymentInfo) Lower(value Lsps1PaymentInfo) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1PaymentInfo](c, value)
}

func (c FfiConverterLsps1PaymentInfo) Write(writer io.Writer, value Lsps1PaymentInfo) {
	FfiConverterOptionalLsps1Bolt11PaymentInfoINSTANCE.Write(writer, value.Bolt11)
	FfiConverterOptionalLsps1OnchainPaymentInfoINSTANCE.Write(writer, value.Onchain)
}

type FfiDestroyerLsps1PaymentInfo struct{}

func (_ FfiDestroyerLsps1PaymentInfo) Destroy(value Lsps1PaymentInfo) {
	value.Destroy()
}

type Lsps2GetInfoResponse struct {
	OpeningFeeParamsMenu []Lsps2OpeningFeeParams
}

func (r *Lsps2GetInfoResponse) Destroy() {
	FfiDestroyerSequenceLsps2OpeningFeeParams{}.Destroy(r.OpeningFeeParamsMenu)
}

type FfiConverterLsps2GetInfoResponse struct{}

var FfiConverterLsps2GetInfoResponseINSTANCE = FfiConverterLsps2GetInfoResponse{}

func (c FfiConverterLsps2GetInfoResponse) Lift(rb RustBufferI) Lsps2GetInfoResponse {
	return LiftFromRustBuffer[Lsps2GetInfoResponse](c, rb)
}

func (c FfiConverterLsps2GetInfoResponse) Read(reader io.Reader) Lsps2GetInfoResponse {
	return Lsps2GetInfoResponse{
		FfiConverterSequenceLsps2OpeningFeeParamsINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps2GetInfoResponse) Lower(value Lsps2GetInfoResponse) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps2GetInfoResponse](c, value)
}

func (c FfiConverterLsps2GetInfoResponse) Write(writer io.Writer, value Lsps2GetInfoResponse) {
	FfiConverterSequenceLsps2OpeningFeeParamsINSTANCE.Write(writer, value.OpeningFeeParamsMenu)
}

type FfiDestroyerLsps2GetInfoResponse struct{}

func (_ FfiDestroyerLsps2GetInfoResponse) Destroy(value Lsps2GetInfoResponse) {
	value.Destroy()
}

type Lsps2OpeningFeeParams struct {
	MinFeeMsat           uint64
	Proportional         uint32
	ValidUntil           LSPSDateTime
	MinLifetime          uint32
	MaxClientToSelfDelay uint32
	MinPaymentSizeMsat   uint64
	MaxPaymentSizeMsat   uint64
	Promise              string
}

func (r *Lsps2OpeningFeeParams) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.MinFeeMsat)
	FfiDestroyerUint32{}.Destroy(r.Proportional)
	FfiDestroyerTypeLSPSDateTime{}.Destroy(r.ValidUntil)
	FfiDestroyerUint32{}.Destroy(r.MinLifetime)
	FfiDestroyerUint32{}.Destroy(r.MaxClientToSelfDelay)
	FfiDestroyerUint64{}.Destroy(r.MinPaymentSizeMsat)
	FfiDestroyerUint64{}.Destroy(r.MaxPaymentSizeMsat)
	FfiDestroyerString{}.Destroy(r.Promise)
}

type FfiConverterLsps2OpeningFeeParams struct{}

var FfiConverterLsps2OpeningFeeParamsINSTANCE = FfiConverterLsps2OpeningFeeParams{}

func (c FfiConverterLsps2OpeningFeeParams) Lift(rb RustBufferI) Lsps2OpeningFeeParams {
	return LiftFromRustBuffer[Lsps2OpeningFeeParams](c, rb)
}

func (c FfiConverterLsps2OpeningFeeParams) Read(reader io.Reader) Lsps2OpeningFeeParams {
	return Lsps2OpeningFeeParams{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterTypeLSPSDateTimeINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps2OpeningFeeParams) Lower(value Lsps2OpeningFeeParams) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps2OpeningFeeParams](c, value)
}

func (c FfiConverterLsps2OpeningFeeParams) Write(writer io.Writer, value Lsps2OpeningFeeParams) {
	FfiConverterUint64INSTANCE.Write(writer, value.MinFeeMsat)
	FfiConverterUint32INSTANCE.Write(writer, value.Proportional)
	FfiConverterTypeLSPSDateTimeINSTANCE.Write(writer, value.ValidUntil)
	FfiConverterUint32INSTANCE.Write(writer, value.MinLifetime)
	FfiConverterUint32INSTANCE.Write(writer, value.MaxClientToSelfDelay)
	FfiConverterUint64INSTANCE.Write(writer, value.MinPaymentSizeMsat)
	FfiConverterUint64INSTANCE.Write(writer, value.MaxPaymentSizeMsat)
	FfiConverterStringINSTANCE.Write(writer, value.Promise)
}

type FfiDestroyerLsps2OpeningFeeParams struct{}

func (_ FfiDestroyerLsps2OpeningFeeParams) Destroy(value Lsps2OpeningFeeParams) {
	value.Destroy()
}

type Lsps2ServiceConfig struct {
	RequireToken               *string
	AdvertiseService           bool
	ChannelOpeningFeePpm       uint32
	ChannelOverProvisioningPpm uint32
	MinChannelOpeningFeeMsat   uint64
	MinChannelLifetime         uint32
	MaxClientToSelfDelay       uint32
	MinPaymentSizeMsat         uint64
	MaxPaymentSizeMsat         uint64
	ClientTrustsLsp            bool
}

func (r *Lsps2ServiceConfig) Destroy() {
	FfiDestroyerOptionalString{}.Destroy(r.RequireToken)
	FfiDestroyerBool{}.Destroy(r.AdvertiseService)
	FfiDestroyerUint32{}.Destroy(r.ChannelOpeningFeePpm)
	FfiDestroyerUint32{}.Destroy(r.ChannelOverProvisioningPpm)
	FfiDestroyerUint64{}.Destroy(r.MinChannelOpeningFeeMsat)
	FfiDestroyerUint32{}.Destroy(r.MinChannelLifetime)
	FfiDestroyerUint32{}.Destroy(r.MaxClientToSelfDelay)
	FfiDestroyerUint64{}.Destroy(r.MinPaymentSizeMsat)
	FfiDestroyerUint64{}.Destroy(r.MaxPaymentSizeMsat)
	FfiDestroyerBool{}.Destroy(r.ClientTrustsLsp)
}

type FfiConverterLsps2ServiceConfig struct{}

var FfiConverterLsps2ServiceConfigINSTANCE = FfiConverterLsps2ServiceConfig{}

func (c FfiConverterLsps2ServiceConfig) Lift(rb RustBufferI) Lsps2ServiceConfig {
	return LiftFromRustBuffer[Lsps2ServiceConfig](c, rb)
}

func (c FfiConverterLsps2ServiceConfig) Read(reader io.Reader) Lsps2ServiceConfig {
	return Lsps2ServiceConfig{
		FfiConverterOptionalStringINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterLsps2ServiceConfig) Lower(value Lsps2ServiceConfig) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps2ServiceConfig](c, value)
}

func (c FfiConverterLsps2ServiceConfig) Write(writer io.Writer, value Lsps2ServiceConfig) {
	FfiConverterOptionalStringINSTANCE.Write(writer, value.RequireToken)
	FfiConverterBoolINSTANCE.Write(writer, value.AdvertiseService)
	FfiConverterUint32INSTANCE.Write(writer, value.ChannelOpeningFeePpm)
	FfiConverterUint32INSTANCE.Write(writer, value.ChannelOverProvisioningPpm)
	FfiConverterUint64INSTANCE.Write(writer, value.MinChannelOpeningFeeMsat)
	FfiConverterUint32INSTANCE.Write(writer, value.MinChannelLifetime)
	FfiConverterUint32INSTANCE.Write(writer, value.MaxClientToSelfDelay)
	FfiConverterUint64INSTANCE.Write(writer, value.MinPaymentSizeMsat)
	FfiConverterUint64INSTANCE.Write(writer, value.MaxPaymentSizeMsat)
	FfiConverterBoolINSTANCE.Write(writer, value.ClientTrustsLsp)
}

type FfiDestroyerLsps2ServiceConfig struct{}

func (_ FfiDestroyerLsps2ServiceConfig) Destroy(value Lsps2ServiceConfig) {
	value.Destroy()
}

type LogRecord struct {
	Level      LogLevel
	Args       string
	ModulePath string
	Line       uint32
}

func (r *LogRecord) Destroy() {
	FfiDestroyerLogLevel{}.Destroy(r.Level)
	FfiDestroyerString{}.Destroy(r.Args)
	FfiDestroyerString{}.Destroy(r.ModulePath)
	FfiDestroyerUint32{}.Destroy(r.Line)
}

type FfiConverterLogRecord struct{}

var FfiConverterLogRecordINSTANCE = FfiConverterLogRecord{}

func (c FfiConverterLogRecord) Lift(rb RustBufferI) LogRecord {
	return LiftFromRustBuffer[LogRecord](c, rb)
}

func (c FfiConverterLogRecord) Read(reader io.Reader) LogRecord {
	return LogRecord{
		FfiConverterLogLevelINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterLogRecord) Lower(value LogRecord) C.RustBuffer {
	return LowerIntoRustBuffer[LogRecord](c, value)
}

func (c FfiConverterLogRecord) Write(writer io.Writer, value LogRecord) {
	FfiConverterLogLevelINSTANCE.Write(writer, value.Level)
	FfiConverterStringINSTANCE.Write(writer, value.Args)
	FfiConverterStringINSTANCE.Write(writer, value.ModulePath)
	FfiConverterUint32INSTANCE.Write(writer, value.Line)
}

type FfiDestroyerLogRecord struct{}

func (_ FfiDestroyerLogRecord) Destroy(value LogRecord) {
	value.Destroy()
}

type NodeAnnouncementInfo struct {
	LastUpdate uint32
	Alias      string
	Addresses  []SocketAddress
}

func (r *NodeAnnouncementInfo) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.LastUpdate)
	FfiDestroyerString{}.Destroy(r.Alias)
	FfiDestroyerSequenceTypeSocketAddress{}.Destroy(r.Addresses)
}

type FfiConverterNodeAnnouncementInfo struct{}

var FfiConverterNodeAnnouncementInfoINSTANCE = FfiConverterNodeAnnouncementInfo{}

func (c FfiConverterNodeAnnouncementInfo) Lift(rb RustBufferI) NodeAnnouncementInfo {
	return LiftFromRustBuffer[NodeAnnouncementInfo](c, rb)
}

func (c FfiConverterNodeAnnouncementInfo) Read(reader io.Reader) NodeAnnouncementInfo {
	return NodeAnnouncementInfo{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterStringINSTANCE.Read(reader),
		FfiConverterSequenceTypeSocketAddressINSTANCE.Read(reader),
	}
}

func (c FfiConverterNodeAnnouncementInfo) Lower(value NodeAnnouncementInfo) C.RustBuffer {
	return LowerIntoRustBuffer[NodeAnnouncementInfo](c, value)
}

func (c FfiConverterNodeAnnouncementInfo) Write(writer io.Writer, value NodeAnnouncementInfo) {
	FfiConverterUint32INSTANCE.Write(writer, value.LastUpdate)
	FfiConverterStringINSTANCE.Write(writer, value.Alias)
	FfiConverterSequenceTypeSocketAddressINSTANCE.Write(writer, value.Addresses)
}

type FfiDestroyerNodeAnnouncementInfo struct{}

func (_ FfiDestroyerNodeAnnouncementInfo) Destroy(value NodeAnnouncementInfo) {
	value.Destroy()
}

type NodeInfo struct {
	Channels         []uint64
	AnnouncementInfo *NodeAnnouncementInfo
}

func (r *NodeInfo) Destroy() {
	FfiDestroyerSequenceUint64{}.Destroy(r.Channels)
	FfiDestroyerOptionalNodeAnnouncementInfo{}.Destroy(r.AnnouncementInfo)
}

type FfiConverterNodeInfo struct{}

var FfiConverterNodeInfoINSTANCE = FfiConverterNodeInfo{}

func (c FfiConverterNodeInfo) Lift(rb RustBufferI) NodeInfo {
	return LiftFromRustBuffer[NodeInfo](c, rb)
}

func (c FfiConverterNodeInfo) Read(reader io.Reader) NodeInfo {
	return NodeInfo{
		FfiConverterSequenceUint64INSTANCE.Read(reader),
		FfiConverterOptionalNodeAnnouncementInfoINSTANCE.Read(reader),
	}
}

func (c FfiConverterNodeInfo) Lower(value NodeInfo) C.RustBuffer {
	return LowerIntoRustBuffer[NodeInfo](c, value)
}

func (c FfiConverterNodeInfo) Write(writer io.Writer, value NodeInfo) {
	FfiConverterSequenceUint64INSTANCE.Write(writer, value.Channels)
	FfiConverterOptionalNodeAnnouncementInfoINSTANCE.Write(writer, value.AnnouncementInfo)
}

type FfiDestroyerNodeInfo struct{}

func (_ FfiDestroyerNodeInfo) Destroy(value NodeInfo) {
	value.Destroy()
}

type NodeStatus struct {
	IsRunning                                bool
	CurrentBestBlock                         BestBlock
	LatestLightningWalletSyncTimestamp       *uint64
	LatestOnchainWalletSyncTimestamp         *uint64
	LatestFeeRateCacheUpdateTimestamp        *uint64
	LatestRgsSnapshotTimestamp               *uint64
	LatestPathfindingScoresSyncTimestamp     *uint64
	LatestNodeAnnouncementBroadcastTimestamp *uint64
	LatestChannelMonitorArchivalHeight       *uint32
}

func (r *NodeStatus) Destroy() {
	FfiDestroyerBool{}.Destroy(r.IsRunning)
	FfiDestroyerBestBlock{}.Destroy(r.CurrentBestBlock)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestLightningWalletSyncTimestamp)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestOnchainWalletSyncTimestamp)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestFeeRateCacheUpdateTimestamp)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestRgsSnapshotTimestamp)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestPathfindingScoresSyncTimestamp)
	FfiDestroyerOptionalUint64{}.Destroy(r.LatestNodeAnnouncementBroadcastTimestamp)
	FfiDestroyerOptionalUint32{}.Destroy(r.LatestChannelMonitorArchivalHeight)
}

type FfiConverterNodeStatus struct{}

var FfiConverterNodeStatusINSTANCE = FfiConverterNodeStatus{}

func (c FfiConverterNodeStatus) Lift(rb RustBufferI) NodeStatus {
	return LiftFromRustBuffer[NodeStatus](c, rb)
}

func (c FfiConverterNodeStatus) Read(reader io.Reader) NodeStatus {
	return NodeStatus{
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBestBlockINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterNodeStatus) Lower(value NodeStatus) C.RustBuffer {
	return LowerIntoRustBuffer[NodeStatus](c, value)
}

func (c FfiConverterNodeStatus) Write(writer io.Writer, value NodeStatus) {
	FfiConverterBoolINSTANCE.Write(writer, value.IsRunning)
	FfiConverterBestBlockINSTANCE.Write(writer, value.CurrentBestBlock)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestLightningWalletSyncTimestamp)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestOnchainWalletSyncTimestamp)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestFeeRateCacheUpdateTimestamp)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestRgsSnapshotTimestamp)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestPathfindingScoresSyncTimestamp)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.LatestNodeAnnouncementBroadcastTimestamp)
	FfiConverterOptionalUint32INSTANCE.Write(writer, value.LatestChannelMonitorArchivalHeight)
}

type FfiDestroyerNodeStatus struct{}

func (_ FfiDestroyerNodeStatus) Destroy(value NodeStatus) {
	value.Destroy()
}

type OutPoint struct {
	Txid Txid
	Vout uint32
}

func (r *OutPoint) Destroy() {
	FfiDestroyerTypeTxid{}.Destroy(r.Txid)
	FfiDestroyerUint32{}.Destroy(r.Vout)
}

type FfiConverterOutPoint struct{}

var FfiConverterOutPointINSTANCE = FfiConverterOutPoint{}

func (c FfiConverterOutPoint) Lift(rb RustBufferI) OutPoint {
	return LiftFromRustBuffer[OutPoint](c, rb)
}

func (c FfiConverterOutPoint) Read(reader io.Reader) OutPoint {
	return OutPoint{
		FfiConverterTypeTxidINSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterOutPoint) Lower(value OutPoint) C.RustBuffer {
	return LowerIntoRustBuffer[OutPoint](c, value)
}

func (c FfiConverterOutPoint) Write(writer io.Writer, value OutPoint) {
	FfiConverterTypeTxidINSTANCE.Write(writer, value.Txid)
	FfiConverterUint32INSTANCE.Write(writer, value.Vout)
}

type FfiDestroyerOutPoint struct{}

func (_ FfiDestroyerOutPoint) Destroy(value OutPoint) {
	value.Destroy()
}

type PaymentDetails struct {
	Id                    PaymentId
	Kind                  PaymentKind
	AmountMsat            *uint64
	FeePaidMsat           *uint64
	Direction             PaymentDirection
	Status                PaymentStatus
	CreatedAt             uint64
	LatestUpdateTimestamp uint64
}

func (r *PaymentDetails) Destroy() {
	FfiDestroyerTypePaymentId{}.Destroy(r.Id)
	FfiDestroyerPaymentKind{}.Destroy(r.Kind)
	FfiDestroyerOptionalUint64{}.Destroy(r.AmountMsat)
	FfiDestroyerOptionalUint64{}.Destroy(r.FeePaidMsat)
	FfiDestroyerPaymentDirection{}.Destroy(r.Direction)
	FfiDestroyerPaymentStatus{}.Destroy(r.Status)
	FfiDestroyerUint64{}.Destroy(r.CreatedAt)
	FfiDestroyerUint64{}.Destroy(r.LatestUpdateTimestamp)
}

type FfiConverterPaymentDetails struct{}

var FfiConverterPaymentDetailsINSTANCE = FfiConverterPaymentDetails{}

func (c FfiConverterPaymentDetails) Lift(rb RustBufferI) PaymentDetails {
	return LiftFromRustBuffer[PaymentDetails](c, rb)
}

func (c FfiConverterPaymentDetails) Read(reader io.Reader) PaymentDetails {
	return PaymentDetails{
		FfiConverterTypePaymentIdINSTANCE.Read(reader),
		FfiConverterPaymentKindINSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterPaymentDirectionINSTANCE.Read(reader),
		FfiConverterPaymentStatusINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
	}
}

func (c FfiConverterPaymentDetails) Lower(value PaymentDetails) C.RustBuffer {
	return LowerIntoRustBuffer[PaymentDetails](c, value)
}

func (c FfiConverterPaymentDetails) Write(writer io.Writer, value PaymentDetails) {
	FfiConverterTypePaymentIdINSTANCE.Write(writer, value.Id)
	FfiConverterPaymentKindINSTANCE.Write(writer, value.Kind)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.AmountMsat)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.FeePaidMsat)
	FfiConverterPaymentDirectionINSTANCE.Write(writer, value.Direction)
	FfiConverterPaymentStatusINSTANCE.Write(writer, value.Status)
	FfiConverterUint64INSTANCE.Write(writer, value.CreatedAt)
	FfiConverterUint64INSTANCE.Write(writer, value.LatestUpdateTimestamp)
}

type FfiDestroyerPaymentDetails struct{}

func (_ FfiDestroyerPaymentDetails) Destroy(value PaymentDetails) {
	value.Destroy()
}

type PeerDetails struct {
	NodeId      PublicKey
	Address     SocketAddress
	IsPersisted bool
	IsConnected bool
}

func (r *PeerDetails) Destroy() {
	FfiDestroyerTypePublicKey{}.Destroy(r.NodeId)
	FfiDestroyerTypeSocketAddress{}.Destroy(r.Address)
	FfiDestroyerBool{}.Destroy(r.IsPersisted)
	FfiDestroyerBool{}.Destroy(r.IsConnected)
}

type FfiConverterPeerDetails struct{}

var FfiConverterPeerDetailsINSTANCE = FfiConverterPeerDetails{}

func (c FfiConverterPeerDetails) Lift(rb RustBufferI) PeerDetails {
	return LiftFromRustBuffer[PeerDetails](c, rb)
}

func (c FfiConverterPeerDetails) Read(reader io.Reader) PeerDetails {
	return PeerDetails{
		FfiConverterTypePublicKeyINSTANCE.Read(reader),
		FfiConverterTypeSocketAddressINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
		FfiConverterBoolINSTANCE.Read(reader),
	}
}

func (c FfiConverterPeerDetails) Lower(value PeerDetails) C.RustBuffer {
	return LowerIntoRustBuffer[PeerDetails](c, value)
}

func (c FfiConverterPeerDetails) Write(writer io.Writer, value PeerDetails) {
	FfiConverterTypePublicKeyINSTANCE.Write(writer, value.NodeId)
	FfiConverterTypeSocketAddressINSTANCE.Write(writer, value.Address)
	FfiConverterBoolINSTANCE.Write(writer, value.IsPersisted)
	FfiConverterBoolINSTANCE.Write(writer, value.IsConnected)
}

type FfiDestroyerPeerDetails struct{}

func (_ FfiDestroyerPeerDetails) Destroy(value PeerDetails) {
	value.Destroy()
}

type ReceivingChannel struct {
	ChannelId     ChannelId
	UserChannelId *UserChannelId
}

func (r *ReceivingChannel) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(r.ChannelId)
	FfiDestroyerOptionalTypeUserChannelId{}.Destroy(r.UserChannelId)
}

type FfiConverterReceivingChannel struct{}

var FfiConverterReceivingChannelINSTANCE = FfiConverterReceivingChannel{}

func (c FfiConverterReceivingChannel) Lift(rb RustBufferI) ReceivingChannel {
	return LiftFromRustBuffer[ReceivingChannel](c, rb)
}

func (c FfiConverterReceivingChannel) Read(reader io.Reader) ReceivingChannel {
	return ReceivingChannel{
		FfiConverterTypeChannelIdINSTANCE.Read(reader),
		FfiConverterOptionalTypeUserChannelIdINSTANCE.Read(reader),
	}
}

func (c FfiConverterReceivingChannel) Lower(value ReceivingChannel) C.RustBuffer {
	return LowerIntoRustBuffer[ReceivingChannel](c, value)
}

func (c FfiConverterReceivingChannel) Write(writer io.Writer, value ReceivingChannel) {
	FfiConverterTypeChannelIdINSTANCE.Write(writer, value.ChannelId)
	FfiConverterOptionalTypeUserChannelIdINSTANCE.Write(writer, value.UserChannelId)
}

type FfiDestroyerReceivingChannel struct{}

func (_ FfiDestroyerReceivingChannel) Destroy(value ReceivingChannel) {
	value.Destroy()
}

type RouteHintHop struct {
	SrcNodeId       PublicKey
	ShortChannelId  uint64
	CltvExpiryDelta uint16
	HtlcMinimumMsat *uint64
	HtlcMaximumMsat *uint64
	Fees            RoutingFees
}

func (r *RouteHintHop) Destroy() {
	FfiDestroyerTypePublicKey{}.Destroy(r.SrcNodeId)
	FfiDestroyerUint64{}.Destroy(r.ShortChannelId)
	FfiDestroyerUint16{}.Destroy(r.CltvExpiryDelta)
	FfiDestroyerOptionalUint64{}.Destroy(r.HtlcMinimumMsat)
	FfiDestroyerOptionalUint64{}.Destroy(r.HtlcMaximumMsat)
	FfiDestroyerRoutingFees{}.Destroy(r.Fees)
}

type FfiConverterRouteHintHop struct{}

var FfiConverterRouteHintHopINSTANCE = FfiConverterRouteHintHop{}

func (c FfiConverterRouteHintHop) Lift(rb RustBufferI) RouteHintHop {
	return LiftFromRustBuffer[RouteHintHop](c, rb)
}

func (c FfiConverterRouteHintHop) Read(reader io.Reader) RouteHintHop {
	return RouteHintHop{
		FfiConverterTypePublicKeyINSTANCE.Read(reader),
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterUint16INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterRoutingFeesINSTANCE.Read(reader),
	}
}

func (c FfiConverterRouteHintHop) Lower(value RouteHintHop) C.RustBuffer {
	return LowerIntoRustBuffer[RouteHintHop](c, value)
}

func (c FfiConverterRouteHintHop) Write(writer io.Writer, value RouteHintHop) {
	FfiConverterTypePublicKeyINSTANCE.Write(writer, value.SrcNodeId)
	FfiConverterUint64INSTANCE.Write(writer, value.ShortChannelId)
	FfiConverterUint16INSTANCE.Write(writer, value.CltvExpiryDelta)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.HtlcMinimumMsat)
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.HtlcMaximumMsat)
	FfiConverterRoutingFeesINSTANCE.Write(writer, value.Fees)
}

type FfiDestroyerRouteHintHop struct{}

func (_ FfiDestroyerRouteHintHop) Destroy(value RouteHintHop) {
	value.Destroy()
}

type RouteParametersConfig struct {
	MaxTotalRoutingFeeMsat          *uint64
	MaxTotalCltvExpiryDelta         uint32
	MaxPathCount                    uint8
	MaxChannelSaturationPowerOfHalf uint8
}

func (r *RouteParametersConfig) Destroy() {
	FfiDestroyerOptionalUint64{}.Destroy(r.MaxTotalRoutingFeeMsat)
	FfiDestroyerUint32{}.Destroy(r.MaxTotalCltvExpiryDelta)
	FfiDestroyerUint8{}.Destroy(r.MaxPathCount)
	FfiDestroyerUint8{}.Destroy(r.MaxChannelSaturationPowerOfHalf)
}

type FfiConverterRouteParametersConfig struct{}

var FfiConverterRouteParametersConfigINSTANCE = FfiConverterRouteParametersConfig{}

func (c FfiConverterRouteParametersConfig) Lift(rb RustBufferI) RouteParametersConfig {
	return LiftFromRustBuffer[RouteParametersConfig](c, rb)
}

func (c FfiConverterRouteParametersConfig) Read(reader io.Reader) RouteParametersConfig {
	return RouteParametersConfig{
		FfiConverterOptionalUint64INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint8INSTANCE.Read(reader),
		FfiConverterUint8INSTANCE.Read(reader),
	}
}

func (c FfiConverterRouteParametersConfig) Lower(value RouteParametersConfig) C.RustBuffer {
	return LowerIntoRustBuffer[RouteParametersConfig](c, value)
}

func (c FfiConverterRouteParametersConfig) Write(writer io.Writer, value RouteParametersConfig) {
	FfiConverterOptionalUint64INSTANCE.Write(writer, value.MaxTotalRoutingFeeMsat)
	FfiConverterUint32INSTANCE.Write(writer, value.MaxTotalCltvExpiryDelta)
	FfiConverterUint8INSTANCE.Write(writer, value.MaxPathCount)
	FfiConverterUint8INSTANCE.Write(writer, value.MaxChannelSaturationPowerOfHalf)
}

type FfiDestroyerRouteParametersConfig struct{}

func (_ FfiDestroyerRouteParametersConfig) Destroy(value RouteParametersConfig) {
	value.Destroy()
}

type RoutingFees struct {
	BaseMsat               uint32
	ProportionalMillionths uint32
}

func (r *RoutingFees) Destroy() {
	FfiDestroyerUint32{}.Destroy(r.BaseMsat)
	FfiDestroyerUint32{}.Destroy(r.ProportionalMillionths)
}

type FfiConverterRoutingFees struct{}

var FfiConverterRoutingFeesINSTANCE = FfiConverterRoutingFees{}

func (c FfiConverterRoutingFees) Lift(rb RustBufferI) RoutingFees {
	return LiftFromRustBuffer[RoutingFees](c, rb)
}

func (c FfiConverterRoutingFees) Read(reader io.Reader) RoutingFees {
	return RoutingFees{
		FfiConverterUint32INSTANCE.Read(reader),
		FfiConverterUint32INSTANCE.Read(reader),
	}
}

func (c FfiConverterRoutingFees) Lower(value RoutingFees) C.RustBuffer {
	return LowerIntoRustBuffer[RoutingFees](c, value)
}

func (c FfiConverterRoutingFees) Write(writer io.Writer, value RoutingFees) {
	FfiConverterUint32INSTANCE.Write(writer, value.BaseMsat)
	FfiConverterUint32INSTANCE.Write(writer, value.ProportionalMillionths)
}

type FfiDestroyerRoutingFees struct{}

func (_ FfiDestroyerRoutingFees) Destroy(value RoutingFees) {
	value.Destroy()
}

type TlvEntry struct {
	Type  uint64
	Value []uint8
}

func (r *TlvEntry) Destroy() {
	FfiDestroyerUint64{}.Destroy(r.Type)
	FfiDestroyerSequenceUint8{}.Destroy(r.Value)
}

type FfiConverterTlvEntry struct{}

var FfiConverterTlvEntryINSTANCE = FfiConverterTlvEntry{}

func (c FfiConverterTlvEntry) Lift(rb RustBufferI) TlvEntry {
	return LiftFromRustBuffer[TlvEntry](c, rb)
}

func (c FfiConverterTlvEntry) Read(reader io.Reader) TlvEntry {
	return TlvEntry{
		FfiConverterUint64INSTANCE.Read(reader),
		FfiConverterSequenceUint8INSTANCE.Read(reader),
	}
}

func (c FfiConverterTlvEntry) Lower(value TlvEntry) C.RustBuffer {
	return LowerIntoRustBuffer[TlvEntry](c, value)
}

func (c FfiConverterTlvEntry) Write(writer io.Writer, value TlvEntry) {
	FfiConverterUint64INSTANCE.Write(writer, value.Type)
	FfiConverterSequenceUint8INSTANCE.Write(writer, value.Value)
}

type FfiDestroyerTlvEntry struct{}

func (_ FfiDestroyerTlvEntry) Destroy(value TlvEntry) {
	value.Destroy()
}

type AsyncPaymentsRole uint

const (
	AsyncPaymentsRoleClient AsyncPaymentsRole = 1
	AsyncPaymentsRoleServer AsyncPaymentsRole = 2
)

type FfiConverterAsyncPaymentsRole struct{}

var FfiConverterAsyncPaymentsRoleINSTANCE = FfiConverterAsyncPaymentsRole{}

func (c FfiConverterAsyncPaymentsRole) Lift(rb RustBufferI) AsyncPaymentsRole {
	return LiftFromRustBuffer[AsyncPaymentsRole](c, rb)
}

func (c FfiConverterAsyncPaymentsRole) Lower(value AsyncPaymentsRole) C.RustBuffer {
	return LowerIntoRustBuffer[AsyncPaymentsRole](c, value)
}
func (FfiConverterAsyncPaymentsRole) Read(reader io.Reader) AsyncPaymentsRole {
	id := readInt32(reader)
	return AsyncPaymentsRole(id)
}

func (FfiConverterAsyncPaymentsRole) Write(writer io.Writer, value AsyncPaymentsRole) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerAsyncPaymentsRole struct{}

func (_ FfiDestroyerAsyncPaymentsRole) Destroy(value AsyncPaymentsRole) {
}

type BalanceSource uint

const (
	BalanceSourceHolderForceClosed       BalanceSource = 1
	BalanceSourceCounterpartyForceClosed BalanceSource = 2
	BalanceSourceCoopClose               BalanceSource = 3
	BalanceSourceHtlc                    BalanceSource = 4
)

type FfiConverterBalanceSource struct{}

var FfiConverterBalanceSourceINSTANCE = FfiConverterBalanceSource{}

func (c FfiConverterBalanceSource) Lift(rb RustBufferI) BalanceSource {
	return LiftFromRustBuffer[BalanceSource](c, rb)
}

func (c FfiConverterBalanceSource) Lower(value BalanceSource) C.RustBuffer {
	return LowerIntoRustBuffer[BalanceSource](c, value)
}
func (FfiConverterBalanceSource) Read(reader io.Reader) BalanceSource {
	id := readInt32(reader)
	return BalanceSource(id)
}

func (FfiConverterBalanceSource) Write(writer io.Writer, value BalanceSource) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerBalanceSource struct{}

func (_ FfiDestroyerBalanceSource) Destroy(value BalanceSource) {
}

type Bolt11InvoiceDescription interface {
	Destroy()
}
type Bolt11InvoiceDescriptionHash struct {
	Hash string
}

func (e Bolt11InvoiceDescriptionHash) Destroy() {
	FfiDestroyerString{}.Destroy(e.Hash)
}

type Bolt11InvoiceDescriptionDirect struct {
	Description string
}

func (e Bolt11InvoiceDescriptionDirect) Destroy() {
	FfiDestroyerString{}.Destroy(e.Description)
}

type FfiConverterBolt11InvoiceDescription struct{}

var FfiConverterBolt11InvoiceDescriptionINSTANCE = FfiConverterBolt11InvoiceDescription{}

func (c FfiConverterBolt11InvoiceDescription) Lift(rb RustBufferI) Bolt11InvoiceDescription {
	return LiftFromRustBuffer[Bolt11InvoiceDescription](c, rb)
}

func (c FfiConverterBolt11InvoiceDescription) Lower(value Bolt11InvoiceDescription) C.RustBuffer {
	return LowerIntoRustBuffer[Bolt11InvoiceDescription](c, value)
}
func (FfiConverterBolt11InvoiceDescription) Read(reader io.Reader) Bolt11InvoiceDescription {
	id := readInt32(reader)
	switch id {
	case 1:
		return Bolt11InvoiceDescriptionHash{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 2:
		return Bolt11InvoiceDescriptionDirect{
			FfiConverterStringINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterBolt11InvoiceDescription.Read()", id))
	}
}

func (FfiConverterBolt11InvoiceDescription) Write(writer io.Writer, value Bolt11InvoiceDescription) {
	switch variant_value := value.(type) {
	case Bolt11InvoiceDescriptionHash:
		writeInt32(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Hash)
	case Bolt11InvoiceDescriptionDirect:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Description)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterBolt11InvoiceDescription.Write", value))
	}
}

type FfiDestroyerBolt11InvoiceDescription struct{}

func (_ FfiDestroyerBolt11InvoiceDescription) Destroy(value Bolt11InvoiceDescription) {
	value.Destroy()
}

type BuildError struct {
	err error
}

// Convience method to turn *BuildError into error
// Avoiding treating nil pointer as non nil error interface
func (err *BuildError) AsError() error {
	if err == nil {
		return nil
	} else {
		return err
	}
}

func (err BuildError) Error() string {
	return fmt.Sprintf("BuildError: %s", err.err.Error())
}

func (err BuildError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrBuildErrorInvalidSeedBytes = fmt.Errorf("BuildErrorInvalidSeedBytes")
var ErrBuildErrorInvalidSeedFile = fmt.Errorf("BuildErrorInvalidSeedFile")
var ErrBuildErrorInvalidSystemTime = fmt.Errorf("BuildErrorInvalidSystemTime")
var ErrBuildErrorInvalidChannelMonitor = fmt.Errorf("BuildErrorInvalidChannelMonitor")
var ErrBuildErrorInvalidListeningAddresses = fmt.Errorf("BuildErrorInvalidListeningAddresses")
var ErrBuildErrorInvalidAnnouncementAddresses = fmt.Errorf("BuildErrorInvalidAnnouncementAddresses")
var ErrBuildErrorInvalidNodeAlias = fmt.Errorf("BuildErrorInvalidNodeAlias")
var ErrBuildErrorRuntimeSetupFailed = fmt.Errorf("BuildErrorRuntimeSetupFailed")
var ErrBuildErrorReadFailed = fmt.Errorf("BuildErrorReadFailed")
var ErrBuildErrorWriteFailed = fmt.Errorf("BuildErrorWriteFailed")
var ErrBuildErrorStoragePathAccessFailed = fmt.Errorf("BuildErrorStoragePathAccessFailed")
var ErrBuildErrorKvStoreSetupFailed = fmt.Errorf("BuildErrorKvStoreSetupFailed")
var ErrBuildErrorWalletSetupFailed = fmt.Errorf("BuildErrorWalletSetupFailed")
var ErrBuildErrorLoggerSetupFailed = fmt.Errorf("BuildErrorLoggerSetupFailed")
var ErrBuildErrorNetworkMismatch = fmt.Errorf("BuildErrorNetworkMismatch")
var ErrBuildErrorAsyncPaymentsConfigMismatch = fmt.Errorf("BuildErrorAsyncPaymentsConfigMismatch")

// Variant structs
type BuildErrorInvalidSeedBytes struct {
	message string
}

func NewBuildErrorInvalidSeedBytes() *BuildError {
	return &BuildError{err: &BuildErrorInvalidSeedBytes{}}
}

func (e BuildErrorInvalidSeedBytes) destroy() {
}

func (err BuildErrorInvalidSeedBytes) Error() string {
	return fmt.Sprintf("InvalidSeedBytes: %s", err.message)
}

func (self BuildErrorInvalidSeedBytes) Is(target error) bool {
	return target == ErrBuildErrorInvalidSeedBytes
}

type BuildErrorInvalidSeedFile struct {
	message string
}

func NewBuildErrorInvalidSeedFile() *BuildError {
	return &BuildError{err: &BuildErrorInvalidSeedFile{}}
}

func (e BuildErrorInvalidSeedFile) destroy() {
}

func (err BuildErrorInvalidSeedFile) Error() string {
	return fmt.Sprintf("InvalidSeedFile: %s", err.message)
}

func (self BuildErrorInvalidSeedFile) Is(target error) bool {
	return target == ErrBuildErrorInvalidSeedFile
}

type BuildErrorInvalidSystemTime struct {
	message string
}

func NewBuildErrorInvalidSystemTime() *BuildError {
	return &BuildError{err: &BuildErrorInvalidSystemTime{}}
}

func (e BuildErrorInvalidSystemTime) destroy() {
}

func (err BuildErrorInvalidSystemTime) Error() string {
	return fmt.Sprintf("InvalidSystemTime: %s", err.message)
}

func (self BuildErrorInvalidSystemTime) Is(target error) bool {
	return target == ErrBuildErrorInvalidSystemTime
}

type BuildErrorInvalidChannelMonitor struct {
	message string
}

func NewBuildErrorInvalidChannelMonitor() *BuildError {
	return &BuildError{err: &BuildErrorInvalidChannelMonitor{}}
}

func (e BuildErrorInvalidChannelMonitor) destroy() {
}

func (err BuildErrorInvalidChannelMonitor) Error() string {
	return fmt.Sprintf("InvalidChannelMonitor: %s", err.message)
}

func (self BuildErrorInvalidChannelMonitor) Is(target error) bool {
	return target == ErrBuildErrorInvalidChannelMonitor
}

type BuildErrorInvalidListeningAddresses struct {
	message string
}

func NewBuildErrorInvalidListeningAddresses() *BuildError {
	return &BuildError{err: &BuildErrorInvalidListeningAddresses{}}
}

func (e BuildErrorInvalidListeningAddresses) destroy() {
}

func (err BuildErrorInvalidListeningAddresses) Error() string {
	return fmt.Sprintf("InvalidListeningAddresses: %s", err.message)
}

func (self BuildErrorInvalidListeningAddresses) Is(target error) bool {
	return target == ErrBuildErrorInvalidListeningAddresses
}

type BuildErrorInvalidAnnouncementAddresses struct {
	message string
}

func NewBuildErrorInvalidAnnouncementAddresses() *BuildError {
	return &BuildError{err: &BuildErrorInvalidAnnouncementAddresses{}}
}

func (e BuildErrorInvalidAnnouncementAddresses) destroy() {
}

func (err BuildErrorInvalidAnnouncementAddresses) Error() string {
	return fmt.Sprintf("InvalidAnnouncementAddresses: %s", err.message)
}

func (self BuildErrorInvalidAnnouncementAddresses) Is(target error) bool {
	return target == ErrBuildErrorInvalidAnnouncementAddresses
}

type BuildErrorInvalidNodeAlias struct {
	message string
}

func NewBuildErrorInvalidNodeAlias() *BuildError {
	return &BuildError{err: &BuildErrorInvalidNodeAlias{}}
}

func (e BuildErrorInvalidNodeAlias) destroy() {
}

func (err BuildErrorInvalidNodeAlias) Error() string {
	return fmt.Sprintf("InvalidNodeAlias: %s", err.message)
}

func (self BuildErrorInvalidNodeAlias) Is(target error) bool {
	return target == ErrBuildErrorInvalidNodeAlias
}

type BuildErrorRuntimeSetupFailed struct {
	message string
}

func NewBuildErrorRuntimeSetupFailed() *BuildError {
	return &BuildError{err: &BuildErrorRuntimeSetupFailed{}}
}

func (e BuildErrorRuntimeSetupFailed) destroy() {
}

func (err BuildErrorRuntimeSetupFailed) Error() string {
	return fmt.Sprintf("RuntimeSetupFailed: %s", err.message)
}

func (self BuildErrorRuntimeSetupFailed) Is(target error) bool {
	return target == ErrBuildErrorRuntimeSetupFailed
}

type BuildErrorReadFailed struct {
	message string
}

func NewBuildErrorReadFailed() *BuildError {
	return &BuildError{err: &BuildErrorReadFailed{}}
}

func (e BuildErrorReadFailed) destroy() {
}

func (err BuildErrorReadFailed) Error() string {
	return fmt.Sprintf("ReadFailed: %s", err.message)
}

func (self BuildErrorReadFailed) Is(target error) bool {
	return target == ErrBuildErrorReadFailed
}

type BuildErrorWriteFailed struct {
	message string
}

func NewBuildErrorWriteFailed() *BuildError {
	return &BuildError{err: &BuildErrorWriteFailed{}}
}

func (e BuildErrorWriteFailed) destroy() {
}

func (err BuildErrorWriteFailed) Error() string {
	return fmt.Sprintf("WriteFailed: %s", err.message)
}

func (self BuildErrorWriteFailed) Is(target error) bool {
	return target == ErrBuildErrorWriteFailed
}

type BuildErrorStoragePathAccessFailed struct {
	message string
}

func NewBuildErrorStoragePathAccessFailed() *BuildError {
	return &BuildError{err: &BuildErrorStoragePathAccessFailed{}}
}

func (e BuildErrorStoragePathAccessFailed) destroy() {
}

func (err BuildErrorStoragePathAccessFailed) Error() string {
	return fmt.Sprintf("StoragePathAccessFailed: %s", err.message)
}

func (self BuildErrorStoragePathAccessFailed) Is(target error) bool {
	return target == ErrBuildErrorStoragePathAccessFailed
}

type BuildErrorKvStoreSetupFailed struct {
	message string
}

func NewBuildErrorKvStoreSetupFailed() *BuildError {
	return &BuildError{err: &BuildErrorKvStoreSetupFailed{}}
}

func (e BuildErrorKvStoreSetupFailed) destroy() {
}

func (err BuildErrorKvStoreSetupFailed) Error() string {
	return fmt.Sprintf("KvStoreSetupFailed: %s", err.message)
}

func (self BuildErrorKvStoreSetupFailed) Is(target error) bool {
	return target == ErrBuildErrorKvStoreSetupFailed
}

type BuildErrorWalletSetupFailed struct {
	message string
}

func NewBuildErrorWalletSetupFailed() *BuildError {
	return &BuildError{err: &BuildErrorWalletSetupFailed{}}
}

func (e BuildErrorWalletSetupFailed) destroy() {
}

func (err BuildErrorWalletSetupFailed) Error() string {
	return fmt.Sprintf("WalletSetupFailed: %s", err.message)
}

func (self BuildErrorWalletSetupFailed) Is(target error) bool {
	return target == ErrBuildErrorWalletSetupFailed
}

type BuildErrorLoggerSetupFailed struct {
	message string
}

func NewBuildErrorLoggerSetupFailed() *BuildError {
	return &BuildError{err: &BuildErrorLoggerSetupFailed{}}
}

func (e BuildErrorLoggerSetupFailed) destroy() {
}

func (err BuildErrorLoggerSetupFailed) Error() string {
	return fmt.Sprintf("LoggerSetupFailed: %s", err.message)
}

func (self BuildErrorLoggerSetupFailed) Is(target error) bool {
	return target == ErrBuildErrorLoggerSetupFailed
}

type BuildErrorNetworkMismatch struct {
	message string
}

func NewBuildErrorNetworkMismatch() *BuildError {
	return &BuildError{err: &BuildErrorNetworkMismatch{}}
}

func (e BuildErrorNetworkMismatch) destroy() {
}

func (err BuildErrorNetworkMismatch) Error() string {
	return fmt.Sprintf("NetworkMismatch: %s", err.message)
}

func (self BuildErrorNetworkMismatch) Is(target error) bool {
	return target == ErrBuildErrorNetworkMismatch
}

type BuildErrorAsyncPaymentsConfigMismatch struct {
	message string
}

func NewBuildErrorAsyncPaymentsConfigMismatch() *BuildError {
	return &BuildError{err: &BuildErrorAsyncPaymentsConfigMismatch{}}
}

func (e BuildErrorAsyncPaymentsConfigMismatch) destroy() {
}

func (err BuildErrorAsyncPaymentsConfigMismatch) Error() string {
	return fmt.Sprintf("AsyncPaymentsConfigMismatch: %s", err.message)
}

func (self BuildErrorAsyncPaymentsConfigMismatch) Is(target error) bool {
	return target == ErrBuildErrorAsyncPaymentsConfigMismatch
}

type FfiConverterBuildError struct{}

var FfiConverterBuildErrorINSTANCE = FfiConverterBuildError{}

func (c FfiConverterBuildError) Lift(eb RustBufferI) *BuildError {
	return LiftFromRustBuffer[*BuildError](c, eb)
}

func (c FfiConverterBuildError) Lower(value *BuildError) C.RustBuffer {
	return LowerIntoRustBuffer[*BuildError](c, value)
}

func (c FfiConverterBuildError) Read(reader io.Reader) *BuildError {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &BuildError{&BuildErrorInvalidSeedBytes{message}}
	case 2:
		return &BuildError{&BuildErrorInvalidSeedFile{message}}
	case 3:
		return &BuildError{&BuildErrorInvalidSystemTime{message}}
	case 4:
		return &BuildError{&BuildErrorInvalidChannelMonitor{message}}
	case 5:
		return &BuildError{&BuildErrorInvalidListeningAddresses{message}}
	case 6:
		return &BuildError{&BuildErrorInvalidAnnouncementAddresses{message}}
	case 7:
		return &BuildError{&BuildErrorInvalidNodeAlias{message}}
	case 8:
		return &BuildError{&BuildErrorRuntimeSetupFailed{message}}
	case 9:
		return &BuildError{&BuildErrorReadFailed{message}}
	case 10:
		return &BuildError{&BuildErrorWriteFailed{message}}
	case 11:
		return &BuildError{&BuildErrorStoragePathAccessFailed{message}}
	case 12:
		return &BuildError{&BuildErrorKvStoreSetupFailed{message}}
	case 13:
		return &BuildError{&BuildErrorWalletSetupFailed{message}}
	case 14:
		return &BuildError{&BuildErrorLoggerSetupFailed{message}}
	case 15:
		return &BuildError{&BuildErrorNetworkMismatch{message}}
	case 16:
		return &BuildError{&BuildErrorAsyncPaymentsConfigMismatch{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterBuildError.Read()", errorID))
	}

}

func (c FfiConverterBuildError) Write(writer io.Writer, value *BuildError) {
	switch variantValue := value.err.(type) {
	case *BuildErrorInvalidSeedBytes:
		writeInt32(writer, 1)
	case *BuildErrorInvalidSeedFile:
		writeInt32(writer, 2)
	case *BuildErrorInvalidSystemTime:
		writeInt32(writer, 3)
	case *BuildErrorInvalidChannelMonitor:
		writeInt32(writer, 4)
	case *BuildErrorInvalidListeningAddresses:
		writeInt32(writer, 5)
	case *BuildErrorInvalidAnnouncementAddresses:
		writeInt32(writer, 6)
	case *BuildErrorInvalidNodeAlias:
		writeInt32(writer, 7)
	case *BuildErrorRuntimeSetupFailed:
		writeInt32(writer, 8)
	case *BuildErrorReadFailed:
		writeInt32(writer, 9)
	case *BuildErrorWriteFailed:
		writeInt32(writer, 10)
	case *BuildErrorStoragePathAccessFailed:
		writeInt32(writer, 11)
	case *BuildErrorKvStoreSetupFailed:
		writeInt32(writer, 12)
	case *BuildErrorWalletSetupFailed:
		writeInt32(writer, 13)
	case *BuildErrorLoggerSetupFailed:
		writeInt32(writer, 14)
	case *BuildErrorNetworkMismatch:
		writeInt32(writer, 15)
	case *BuildErrorAsyncPaymentsConfigMismatch:
		writeInt32(writer, 16)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterBuildError.Write", value))
	}
}

type FfiDestroyerBuildError struct{}

func (_ FfiDestroyerBuildError) Destroy(value *BuildError) {
	switch variantValue := value.err.(type) {
	case BuildErrorInvalidSeedBytes:
		variantValue.destroy()
	case BuildErrorInvalidSeedFile:
		variantValue.destroy()
	case BuildErrorInvalidSystemTime:
		variantValue.destroy()
	case BuildErrorInvalidChannelMonitor:
		variantValue.destroy()
	case BuildErrorInvalidListeningAddresses:
		variantValue.destroy()
	case BuildErrorInvalidAnnouncementAddresses:
		variantValue.destroy()
	case BuildErrorInvalidNodeAlias:
		variantValue.destroy()
	case BuildErrorRuntimeSetupFailed:
		variantValue.destroy()
	case BuildErrorReadFailed:
		variantValue.destroy()
	case BuildErrorWriteFailed:
		variantValue.destroy()
	case BuildErrorStoragePathAccessFailed:
		variantValue.destroy()
	case BuildErrorKvStoreSetupFailed:
		variantValue.destroy()
	case BuildErrorWalletSetupFailed:
		variantValue.destroy()
	case BuildErrorLoggerSetupFailed:
		variantValue.destroy()
	case BuildErrorNetworkMismatch:
		variantValue.destroy()
	case BuildErrorAsyncPaymentsConfigMismatch:
		variantValue.destroy()
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiDestroyerBuildError.Destroy", value))
	}
}

type ClosureReason interface {
	Destroy()
}
type ClosureReasonCounterpartyForceClosed struct {
	PeerMsg UntrustedString
}

func (e ClosureReasonCounterpartyForceClosed) Destroy() {
	FfiDestroyerTypeUntrustedString{}.Destroy(e.PeerMsg)
}

type ClosureReasonHolderForceClosed struct {
	BroadcastedLatestTxn *bool
	Message              string
}

func (e ClosureReasonHolderForceClosed) Destroy() {
	FfiDestroyerOptionalBool{}.Destroy(e.BroadcastedLatestTxn)
	FfiDestroyerString{}.Destroy(e.Message)
}

type ClosureReasonLegacyCooperativeClosure struct {
}

func (e ClosureReasonLegacyCooperativeClosure) Destroy() {
}

type ClosureReasonCounterpartyInitiatedCooperativeClosure struct {
}

func (e ClosureReasonCounterpartyInitiatedCooperativeClosure) Destroy() {
}

type ClosureReasonLocallyInitiatedCooperativeClosure struct {
}

func (e ClosureReasonLocallyInitiatedCooperativeClosure) Destroy() {
}

type ClosureReasonCommitmentTxConfirmed struct {
}

func (e ClosureReasonCommitmentTxConfirmed) Destroy() {
}

type ClosureReasonFundingTimedOut struct {
}

func (e ClosureReasonFundingTimedOut) Destroy() {
}

type ClosureReasonProcessingError struct {
	Err string
}

func (e ClosureReasonProcessingError) Destroy() {
	FfiDestroyerString{}.Destroy(e.Err)
}

type ClosureReasonDisconnectedPeer struct {
}

func (e ClosureReasonDisconnectedPeer) Destroy() {
}

type ClosureReasonOutdatedChannelManager struct {
}

func (e ClosureReasonOutdatedChannelManager) Destroy() {
}

type ClosureReasonCounterpartyCoopClosedUnfundedChannel struct {
}

func (e ClosureReasonCounterpartyCoopClosedUnfundedChannel) Destroy() {
}

type ClosureReasonLocallyCoopClosedUnfundedChannel struct {
}

func (e ClosureReasonLocallyCoopClosedUnfundedChannel) Destroy() {
}

type ClosureReasonFundingBatchClosure struct {
}

func (e ClosureReasonFundingBatchClosure) Destroy() {
}

type ClosureReasonHtlCsTimedOut struct {
	PaymentHash *PaymentHash
}

func (e ClosureReasonHtlCsTimedOut) Destroy() {
	FfiDestroyerOptionalTypePaymentHash{}.Destroy(e.PaymentHash)
}

type ClosureReasonPeerFeerateTooLow struct {
	PeerFeerateSatPerKw     uint32
	RequiredFeerateSatPerKw uint32
}

func (e ClosureReasonPeerFeerateTooLow) Destroy() {
	FfiDestroyerUint32{}.Destroy(e.PeerFeerateSatPerKw)
	FfiDestroyerUint32{}.Destroy(e.RequiredFeerateSatPerKw)
}

type FfiConverterClosureReason struct{}

var FfiConverterClosureReasonINSTANCE = FfiConverterClosureReason{}

func (c FfiConverterClosureReason) Lift(rb RustBufferI) ClosureReason {
	return LiftFromRustBuffer[ClosureReason](c, rb)
}

func (c FfiConverterClosureReason) Lower(value ClosureReason) C.RustBuffer {
	return LowerIntoRustBuffer[ClosureReason](c, value)
}
func (FfiConverterClosureReason) Read(reader io.Reader) ClosureReason {
	id := readInt32(reader)
	switch id {
	case 1:
		return ClosureReasonCounterpartyForceClosed{
			FfiConverterTypeUntrustedStringINSTANCE.Read(reader),
		}
	case 2:
		return ClosureReasonHolderForceClosed{
			FfiConverterOptionalBoolINSTANCE.Read(reader),
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 3:
		return ClosureReasonLegacyCooperativeClosure{}
	case 4:
		return ClosureReasonCounterpartyInitiatedCooperativeClosure{}
	case 5:
		return ClosureReasonLocallyInitiatedCooperativeClosure{}
	case 6:
		return ClosureReasonCommitmentTxConfirmed{}
	case 7:
		return ClosureReasonFundingTimedOut{}
	case 8:
		return ClosureReasonProcessingError{
			FfiConverterStringINSTANCE.Read(reader),
		}
	case 9:
		return ClosureReasonDisconnectedPeer{}
	case 10:
		return ClosureReasonOutdatedChannelManager{}
	case 11:
		return ClosureReasonCounterpartyCoopClosedUnfundedChannel{}
	case 12:
		return ClosureReasonLocallyCoopClosedUnfundedChannel{}
	case 13:
		return ClosureReasonFundingBatchClosure{}
	case 14:
		return ClosureReasonHtlCsTimedOut{
			FfiConverterOptionalTypePaymentHashINSTANCE.Read(reader),
		}
	case 15:
		return ClosureReasonPeerFeerateTooLow{
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterClosureReason.Read()", id))
	}
}

func (FfiConverterClosureReason) Write(writer io.Writer, value ClosureReason) {
	switch variant_value := value.(type) {
	case ClosureReasonCounterpartyForceClosed:
		writeInt32(writer, 1)
		FfiConverterTypeUntrustedStringINSTANCE.Write(writer, variant_value.PeerMsg)
	case ClosureReasonHolderForceClosed:
		writeInt32(writer, 2)
		FfiConverterOptionalBoolINSTANCE.Write(writer, variant_value.BroadcastedLatestTxn)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Message)
	case ClosureReasonLegacyCooperativeClosure:
		writeInt32(writer, 3)
	case ClosureReasonCounterpartyInitiatedCooperativeClosure:
		writeInt32(writer, 4)
	case ClosureReasonLocallyInitiatedCooperativeClosure:
		writeInt32(writer, 5)
	case ClosureReasonCommitmentTxConfirmed:
		writeInt32(writer, 6)
	case ClosureReasonFundingTimedOut:
		writeInt32(writer, 7)
	case ClosureReasonProcessingError:
		writeInt32(writer, 8)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Err)
	case ClosureReasonDisconnectedPeer:
		writeInt32(writer, 9)
	case ClosureReasonOutdatedChannelManager:
		writeInt32(writer, 10)
	case ClosureReasonCounterpartyCoopClosedUnfundedChannel:
		writeInt32(writer, 11)
	case ClosureReasonLocallyCoopClosedUnfundedChannel:
		writeInt32(writer, 12)
	case ClosureReasonFundingBatchClosure:
		writeInt32(writer, 13)
	case ClosureReasonHtlCsTimedOut:
		writeInt32(writer, 14)
		FfiConverterOptionalTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
	case ClosureReasonPeerFeerateTooLow:
		writeInt32(writer, 15)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.PeerFeerateSatPerKw)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.RequiredFeerateSatPerKw)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterClosureReason.Write", value))
	}
}

type FfiDestroyerClosureReason struct{}

func (_ FfiDestroyerClosureReason) Destroy(value ClosureReason) {
	value.Destroy()
}

type ConfirmationStatus interface {
	Destroy()
}
type ConfirmationStatusConfirmed struct {
	BlockHash BlockHash
	Height    uint32
	Timestamp uint64
}

func (e ConfirmationStatusConfirmed) Destroy() {
	FfiDestroyerTypeBlockHash{}.Destroy(e.BlockHash)
	FfiDestroyerUint32{}.Destroy(e.Height)
	FfiDestroyerUint64{}.Destroy(e.Timestamp)
}

type ConfirmationStatusUnconfirmed struct {
}

func (e ConfirmationStatusUnconfirmed) Destroy() {
}

type FfiConverterConfirmationStatus struct{}

var FfiConverterConfirmationStatusINSTANCE = FfiConverterConfirmationStatus{}

func (c FfiConverterConfirmationStatus) Lift(rb RustBufferI) ConfirmationStatus {
	return LiftFromRustBuffer[ConfirmationStatus](c, rb)
}

func (c FfiConverterConfirmationStatus) Lower(value ConfirmationStatus) C.RustBuffer {
	return LowerIntoRustBuffer[ConfirmationStatus](c, value)
}
func (FfiConverterConfirmationStatus) Read(reader io.Reader) ConfirmationStatus {
	id := readInt32(reader)
	switch id {
	case 1:
		return ConfirmationStatusConfirmed{
			FfiConverterTypeBlockHashINSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
		}
	case 2:
		return ConfirmationStatusUnconfirmed{}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterConfirmationStatus.Read()", id))
	}
}

func (FfiConverterConfirmationStatus) Write(writer io.Writer, value ConfirmationStatus) {
	switch variant_value := value.(type) {
	case ConfirmationStatusConfirmed:
		writeInt32(writer, 1)
		FfiConverterTypeBlockHashINSTANCE.Write(writer, variant_value.BlockHash)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.Height)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.Timestamp)
	case ConfirmationStatusUnconfirmed:
		writeInt32(writer, 2)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterConfirmationStatus.Write", value))
	}
}

type FfiDestroyerConfirmationStatus struct{}

func (_ FfiDestroyerConfirmationStatus) Destroy(value ConfirmationStatus) {
	value.Destroy()
}

type Currency uint

const (
	CurrencyBitcoin        Currency = 1
	CurrencyBitcoinTestnet Currency = 2
	CurrencyRegtest        Currency = 3
	CurrencySimnet         Currency = 4
	CurrencySignet         Currency = 5
)

type FfiConverterCurrency struct{}

var FfiConverterCurrencyINSTANCE = FfiConverterCurrency{}

func (c FfiConverterCurrency) Lift(rb RustBufferI) Currency {
	return LiftFromRustBuffer[Currency](c, rb)
}

func (c FfiConverterCurrency) Lower(value Currency) C.RustBuffer {
	return LowerIntoRustBuffer[Currency](c, value)
}
func (FfiConverterCurrency) Read(reader io.Reader) Currency {
	id := readInt32(reader)
	return Currency(id)
}

func (FfiConverterCurrency) Write(writer io.Writer, value Currency) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerCurrency struct{}

func (_ FfiDestroyerCurrency) Destroy(value Currency) {
}

type Event interface {
	Destroy()
}
type EventPaymentSuccessful struct {
	PaymentId       *PaymentId
	PaymentHash     PaymentHash
	PaymentPreimage *PaymentPreimage
	FeePaidMsat     *uint64
}

func (e EventPaymentSuccessful) Destroy() {
	FfiDestroyerOptionalTypePaymentId{}.Destroy(e.PaymentId)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.PaymentPreimage)
	FfiDestroyerOptionalUint64{}.Destroy(e.FeePaidMsat)
}

type EventPaymentFailed struct {
	PaymentId   *PaymentId
	PaymentHash *PaymentHash
	Reason      *PaymentFailureReason
}

func (e EventPaymentFailed) Destroy() {
	FfiDestroyerOptionalTypePaymentId{}.Destroy(e.PaymentId)
	FfiDestroyerOptionalTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerOptionalPaymentFailureReason{}.Destroy(e.Reason)
}

type EventPaymentReceived struct {
	PaymentId     *PaymentId
	PaymentHash   PaymentHash
	AmountMsat    uint64
	CustomRecords []CustomTlvRecord
}

func (e EventPaymentReceived) Destroy() {
	FfiDestroyerOptionalTypePaymentId{}.Destroy(e.PaymentId)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerUint64{}.Destroy(e.AmountMsat)
	FfiDestroyerSequenceCustomTlvRecord{}.Destroy(e.CustomRecords)
}

type EventPaymentClaimable struct {
	PaymentId           PaymentId
	PaymentHash         PaymentHash
	ClaimableAmountMsat uint64
	ClaimDeadline       *uint32
	CustomRecords       []CustomTlvRecord
	ReceivingChannels   []ReceivingChannel
}

func (e EventPaymentClaimable) Destroy() {
	FfiDestroyerTypePaymentId{}.Destroy(e.PaymentId)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerUint64{}.Destroy(e.ClaimableAmountMsat)
	FfiDestroyerOptionalUint32{}.Destroy(e.ClaimDeadline)
	FfiDestroyerSequenceCustomTlvRecord{}.Destroy(e.CustomRecords)
	FfiDestroyerSequenceReceivingChannel{}.Destroy(e.ReceivingChannels)
}

type EventPaymentForwarded struct {
	PrevChannelId               ChannelId
	NextChannelId               ChannelId
	PrevUserChannelId           *UserChannelId
	NextUserChannelId           *UserChannelId
	PrevNodeId                  *PublicKey
	NextNodeId                  *PublicKey
	TotalFeeEarnedMsat          *uint64
	SkimmedFeeMsat              *uint64
	ClaimFromOnchainTx          bool
	OutboundAmountForwardedMsat *uint64
}

func (e EventPaymentForwarded) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.PrevChannelId)
	FfiDestroyerTypeChannelId{}.Destroy(e.NextChannelId)
	FfiDestroyerOptionalTypeUserChannelId{}.Destroy(e.PrevUserChannelId)
	FfiDestroyerOptionalTypeUserChannelId{}.Destroy(e.NextUserChannelId)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.PrevNodeId)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.NextNodeId)
	FfiDestroyerOptionalUint64{}.Destroy(e.TotalFeeEarnedMsat)
	FfiDestroyerOptionalUint64{}.Destroy(e.SkimmedFeeMsat)
	FfiDestroyerBool{}.Destroy(e.ClaimFromOnchainTx)
	FfiDestroyerOptionalUint64{}.Destroy(e.OutboundAmountForwardedMsat)
}

type EventChannelPending struct {
	ChannelId                ChannelId
	UserChannelId            UserChannelId
	FormerTemporaryChannelId ChannelId
	CounterpartyNodeId       PublicKey
	FundingTxo               OutPoint
}

func (e EventChannelPending) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeUserChannelId{}.Destroy(e.UserChannelId)
	FfiDestroyerTypeChannelId{}.Destroy(e.FormerTemporaryChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOutPoint{}.Destroy(e.FundingTxo)
}

type EventChannelReady struct {
	ChannelId          ChannelId
	UserChannelId      UserChannelId
	CounterpartyNodeId *PublicKey
	FundingTxo         *OutPoint
}

func (e EventChannelReady) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeUserChannelId{}.Destroy(e.UserChannelId)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalOutPoint{}.Destroy(e.FundingTxo)
}

type EventChannelClosed struct {
	ChannelId          ChannelId
	UserChannelId      UserChannelId
	CounterpartyNodeId *PublicKey
	Reason             *ClosureReason
}

func (e EventChannelClosed) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeUserChannelId{}.Destroy(e.UserChannelId)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalClosureReason{}.Destroy(e.Reason)
}

type EventSplicePending struct {
	ChannelId          ChannelId
	UserChannelId      UserChannelId
	CounterpartyNodeId PublicKey
	NewFundingTxo      OutPoint
}

func (e EventSplicePending) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeUserChannelId{}.Destroy(e.UserChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOutPoint{}.Destroy(e.NewFundingTxo)
}

type EventSpliceFailed struct {
	ChannelId           ChannelId
	UserChannelId       UserChannelId
	CounterpartyNodeId  PublicKey
	AbandonedFundingTxo *OutPoint
}

func (e EventSpliceFailed) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeUserChannelId{}.Destroy(e.UserChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalOutPoint{}.Destroy(e.AbandonedFundingTxo)
}

type FfiConverterEvent struct{}

var FfiConverterEventINSTANCE = FfiConverterEvent{}

func (c FfiConverterEvent) Lift(rb RustBufferI) Event {
	return LiftFromRustBuffer[Event](c, rb)
}

func (c FfiConverterEvent) Lower(value Event) C.RustBuffer {
	return LowerIntoRustBuffer[Event](c, value)
}
func (FfiConverterEvent) Read(reader io.Reader) Event {
	id := readInt32(reader)
	switch id {
	case 1:
		return EventPaymentSuccessful{
			FfiConverterOptionalTypePaymentIdINSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
		}
	case 2:
		return EventPaymentFailed{
			FfiConverterOptionalTypePaymentIdINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalPaymentFailureReasonINSTANCE.Read(reader),
		}
	case 3:
		return EventPaymentReceived{
			FfiConverterOptionalTypePaymentIdINSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterSequenceCustomTlvRecordINSTANCE.Read(reader),
		}
	case 4:
		return EventPaymentClaimable{
			FfiConverterTypePaymentIdINSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterOptionalUint32INSTANCE.Read(reader),
			FfiConverterSequenceCustomTlvRecordINSTANCE.Read(reader),
			FfiConverterSequenceReceivingChannelINSTANCE.Read(reader),
		}
	case 5:
		return EventPaymentForwarded{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterOptionalTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterOptionalTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
		}
	case 6:
		return EventChannelPending{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOutPointINSTANCE.Read(reader),
		}
	case 7:
		return EventChannelReady{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalOutPointINSTANCE.Read(reader),
		}
	case 8:
		return EventChannelClosed{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalClosureReasonINSTANCE.Read(reader),
		}
	case 9:
		return EventSplicePending{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOutPointINSTANCE.Read(reader),
		}
	case 10:
		return EventSpliceFailed{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeUserChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalOutPointINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterEvent.Read()", id))
	}
}

func (FfiConverterEvent) Write(writer io.Writer, value Event) {
	switch variant_value := value.(type) {
	case EventPaymentSuccessful:
		writeInt32(writer, 1)
		FfiConverterOptionalTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.PaymentPreimage)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.FeePaidMsat)
	case EventPaymentFailed:
		writeInt32(writer, 2)
		FfiConverterOptionalTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
		FfiConverterOptionalTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterOptionalPaymentFailureReasonINSTANCE.Write(writer, variant_value.Reason)
	case EventPaymentReceived:
		writeInt32(writer, 3)
		FfiConverterOptionalTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountMsat)
		FfiConverterSequenceCustomTlvRecordINSTANCE.Write(writer, variant_value.CustomRecords)
	case EventPaymentClaimable:
		writeInt32(writer, 4)
		FfiConverterTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.ClaimableAmountMsat)
		FfiConverterOptionalUint32INSTANCE.Write(writer, variant_value.ClaimDeadline)
		FfiConverterSequenceCustomTlvRecordINSTANCE.Write(writer, variant_value.CustomRecords)
		FfiConverterSequenceReceivingChannelINSTANCE.Write(writer, variant_value.ReceivingChannels)
	case EventPaymentForwarded:
		writeInt32(writer, 5)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.PrevChannelId)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.NextChannelId)
		FfiConverterOptionalTypeUserChannelIdINSTANCE.Write(writer, variant_value.PrevUserChannelId)
		FfiConverterOptionalTypeUserChannelIdINSTANCE.Write(writer, variant_value.NextUserChannelId)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.PrevNodeId)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.NextNodeId)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.TotalFeeEarnedMsat)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.SkimmedFeeMsat)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.ClaimFromOnchainTx)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.OutboundAmountForwardedMsat)
	case EventChannelPending:
		writeInt32(writer, 6)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, variant_value.UserChannelId)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.FormerTemporaryChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOutPointINSTANCE.Write(writer, variant_value.FundingTxo)
	case EventChannelReady:
		writeInt32(writer, 7)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, variant_value.UserChannelId)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalOutPointINSTANCE.Write(writer, variant_value.FundingTxo)
	case EventChannelClosed:
		writeInt32(writer, 8)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, variant_value.UserChannelId)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalClosureReasonINSTANCE.Write(writer, variant_value.Reason)
	case EventSplicePending:
		writeInt32(writer, 9)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, variant_value.UserChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOutPointINSTANCE.Write(writer, variant_value.NewFundingTxo)
	case EventSpliceFailed:
		writeInt32(writer, 10)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, variant_value.UserChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalOutPointINSTANCE.Write(writer, variant_value.AbandonedFundingTxo)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterEvent.Write", value))
	}
}

type FfiDestroyerEvent struct{}

func (_ FfiDestroyerEvent) Destroy(value Event) {
	value.Destroy()
}

type Lsps1PaymentState uint

const (
	Lsps1PaymentStateExpectPayment Lsps1PaymentState = 1
	Lsps1PaymentStatePaid          Lsps1PaymentState = 2
	Lsps1PaymentStateRefunded      Lsps1PaymentState = 3
)

type FfiConverterLsps1PaymentState struct{}

var FfiConverterLsps1PaymentStateINSTANCE = FfiConverterLsps1PaymentState{}

func (c FfiConverterLsps1PaymentState) Lift(rb RustBufferI) Lsps1PaymentState {
	return LiftFromRustBuffer[Lsps1PaymentState](c, rb)
}

func (c FfiConverterLsps1PaymentState) Lower(value Lsps1PaymentState) C.RustBuffer {
	return LowerIntoRustBuffer[Lsps1PaymentState](c, value)
}
func (FfiConverterLsps1PaymentState) Read(reader io.Reader) Lsps1PaymentState {
	id := readInt32(reader)
	return Lsps1PaymentState(id)
}

func (FfiConverterLsps1PaymentState) Write(writer io.Writer, value Lsps1PaymentState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerLsps1PaymentState struct{}

func (_ FfiDestroyerLsps1PaymentState) Destroy(value Lsps1PaymentState) {
}

type LightningBalance interface {
	Destroy()
}
type LightningBalanceClaimableOnChannelClose struct {
	ChannelId                        ChannelId
	CounterpartyNodeId               PublicKey
	FundingTxId                      Txid
	FundingTxIndex                   uint16
	AmountSatoshis                   uint64
	TransactionFeeSatoshis           uint64
	OutboundPaymentHtlcRoundedMsat   uint64
	OutboundForwardedHtlcRoundedMsat uint64
	InboundClaimingHtlcRoundedMsat   uint64
	InboundHtlcRoundedMsat           uint64
}

func (e LightningBalanceClaimableOnChannelClose) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerUint64{}.Destroy(e.TransactionFeeSatoshis)
	FfiDestroyerUint64{}.Destroy(e.OutboundPaymentHtlcRoundedMsat)
	FfiDestroyerUint64{}.Destroy(e.OutboundForwardedHtlcRoundedMsat)
	FfiDestroyerUint64{}.Destroy(e.InboundClaimingHtlcRoundedMsat)
	FfiDestroyerUint64{}.Destroy(e.InboundHtlcRoundedMsat)
}

type LightningBalanceClaimableAwaitingConfirmations struct {
	ChannelId          ChannelId
	CounterpartyNodeId PublicKey
	FundingTxId        Txid
	FundingTxIndex     uint16
	AmountSatoshis     uint64
	ConfirmationHeight uint32
	Source             BalanceSource
}

func (e LightningBalanceClaimableAwaitingConfirmations) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerUint32{}.Destroy(e.ConfirmationHeight)
	FfiDestroyerBalanceSource{}.Destroy(e.Source)
}

type LightningBalanceContentiousClaimable struct {
	ChannelId          ChannelId
	CounterpartyNodeId PublicKey
	FundingTxId        Txid
	FundingTxIndex     uint16
	AmountSatoshis     uint64
	TimeoutHeight      uint32
	PaymentHash        PaymentHash
	PaymentPreimage    PaymentPreimage
}

func (e LightningBalanceContentiousClaimable) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerUint32{}.Destroy(e.TimeoutHeight)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerTypePaymentPreimage{}.Destroy(e.PaymentPreimage)
}

type LightningBalanceMaybeTimeoutClaimableHtlc struct {
	ChannelId          ChannelId
	CounterpartyNodeId PublicKey
	FundingTxId        Txid
	FundingTxIndex     uint16
	AmountSatoshis     uint64
	ClaimableHeight    uint32
	PaymentHash        PaymentHash
	OutboundPayment    bool
}

func (e LightningBalanceMaybeTimeoutClaimableHtlc) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerUint32{}.Destroy(e.ClaimableHeight)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
	FfiDestroyerBool{}.Destroy(e.OutboundPayment)
}

type LightningBalanceMaybePreimageClaimableHtlc struct {
	ChannelId          ChannelId
	CounterpartyNodeId PublicKey
	FundingTxId        Txid
	FundingTxIndex     uint16
	AmountSatoshis     uint64
	ExpiryHeight       uint32
	PaymentHash        PaymentHash
}

func (e LightningBalanceMaybePreimageClaimableHtlc) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerUint32{}.Destroy(e.ExpiryHeight)
	FfiDestroyerTypePaymentHash{}.Destroy(e.PaymentHash)
}

type LightningBalanceCounterpartyRevokedOutputClaimable struct {
	ChannelId          ChannelId
	CounterpartyNodeId PublicKey
	FundingTxId        Txid
	FundingTxIndex     uint16
	AmountSatoshis     uint64
}

func (e LightningBalanceCounterpartyRevokedOutputClaimable) Destroy() {
	FfiDestroyerTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerUint16{}.Destroy(e.FundingTxIndex)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
}

type FfiConverterLightningBalance struct{}

var FfiConverterLightningBalanceINSTANCE = FfiConverterLightningBalance{}

func (c FfiConverterLightningBalance) Lift(rb RustBufferI) LightningBalance {
	return LiftFromRustBuffer[LightningBalance](c, rb)
}

func (c FfiConverterLightningBalance) Lower(value LightningBalance) C.RustBuffer {
	return LowerIntoRustBuffer[LightningBalance](c, value)
}
func (FfiConverterLightningBalance) Read(reader io.Reader) LightningBalance {
	id := readInt32(reader)
	switch id {
	case 1:
		return LightningBalanceClaimableOnChannelClose{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
		}
	case 2:
		return LightningBalanceClaimableAwaitingConfirmations{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterBalanceSourceINSTANCE.Read(reader),
		}
	case 3:
		return LightningBalanceContentiousClaimable{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterTypePaymentPreimageINSTANCE.Read(reader),
		}
	case 4:
		return LightningBalanceMaybeTimeoutClaimableHtlc{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterBoolINSTANCE.Read(reader),
		}
	case 5:
		return LightningBalanceMaybePreimageClaimableHtlc{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
		}
	case 6:
		return LightningBalanceCounterpartyRevokedOutputClaimable{
			FfiConverterTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypePublicKeyINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint16INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterLightningBalance.Read()", id))
	}
}

func (FfiConverterLightningBalance) Write(writer io.Writer, value LightningBalance) {
	switch variant_value := value.(type) {
	case LightningBalanceClaimableOnChannelClose:
		writeInt32(writer, 1)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.TransactionFeeSatoshis)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.OutboundPaymentHtlcRoundedMsat)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.OutboundForwardedHtlcRoundedMsat)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.InboundClaimingHtlcRoundedMsat)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.InboundHtlcRoundedMsat)
	case LightningBalanceClaimableAwaitingConfirmations:
		writeInt32(writer, 2)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.ConfirmationHeight)
		FfiConverterBalanceSourceINSTANCE.Write(writer, variant_value.Source)
	case LightningBalanceContentiousClaimable:
		writeInt32(writer, 3)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.TimeoutHeight)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterTypePaymentPreimageINSTANCE.Write(writer, variant_value.PaymentPreimage)
	case LightningBalanceMaybeTimeoutClaimableHtlc:
		writeInt32(writer, 4)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.ClaimableHeight)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
		FfiConverterBoolINSTANCE.Write(writer, variant_value.OutboundPayment)
	case LightningBalanceMaybePreimageClaimableHtlc:
		writeInt32(writer, 5)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.ExpiryHeight)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.PaymentHash)
	case LightningBalanceCounterpartyRevokedOutputClaimable:
		writeInt32(writer, 6)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterLightningBalance.Write", value))
	}
}

type FfiDestroyerLightningBalance struct{}

func (_ FfiDestroyerLightningBalance) Destroy(value LightningBalance) {
	value.Destroy()
}

type LogLevel uint

const (
	LogLevelGossip LogLevel = 1
	LogLevelTrace  LogLevel = 2
	LogLevelDebug  LogLevel = 3
	LogLevelInfo   LogLevel = 4
	LogLevelWarn   LogLevel = 5
	LogLevelError  LogLevel = 6
)

type FfiConverterLogLevel struct{}

var FfiConverterLogLevelINSTANCE = FfiConverterLogLevel{}

func (c FfiConverterLogLevel) Lift(rb RustBufferI) LogLevel {
	return LiftFromRustBuffer[LogLevel](c, rb)
}

func (c FfiConverterLogLevel) Lower(value LogLevel) C.RustBuffer {
	return LowerIntoRustBuffer[LogLevel](c, value)
}
func (FfiConverterLogLevel) Read(reader io.Reader) LogLevel {
	id := readInt32(reader)
	return LogLevel(id)
}

func (FfiConverterLogLevel) Write(writer io.Writer, value LogLevel) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerLogLevel struct{}

func (_ FfiDestroyerLogLevel) Destroy(value LogLevel) {
}

type MaxDustHtlcExposure interface {
	Destroy()
}
type MaxDustHtlcExposureFixedLimit struct {
	LimitMsat uint64
}

func (e MaxDustHtlcExposureFixedLimit) Destroy() {
	FfiDestroyerUint64{}.Destroy(e.LimitMsat)
}

type MaxDustHtlcExposureFeeRateMultiplier struct {
	Multiplier uint64
}

func (e MaxDustHtlcExposureFeeRateMultiplier) Destroy() {
	FfiDestroyerUint64{}.Destroy(e.Multiplier)
}

type FfiConverterMaxDustHtlcExposure struct{}

var FfiConverterMaxDustHtlcExposureINSTANCE = FfiConverterMaxDustHtlcExposure{}

func (c FfiConverterMaxDustHtlcExposure) Lift(rb RustBufferI) MaxDustHtlcExposure {
	return LiftFromRustBuffer[MaxDustHtlcExposure](c, rb)
}

func (c FfiConverterMaxDustHtlcExposure) Lower(value MaxDustHtlcExposure) C.RustBuffer {
	return LowerIntoRustBuffer[MaxDustHtlcExposure](c, value)
}
func (FfiConverterMaxDustHtlcExposure) Read(reader io.Reader) MaxDustHtlcExposure {
	id := readInt32(reader)
	switch id {
	case 1:
		return MaxDustHtlcExposureFixedLimit{
			FfiConverterUint64INSTANCE.Read(reader),
		}
	case 2:
		return MaxDustHtlcExposureFeeRateMultiplier{
			FfiConverterUint64INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterMaxDustHtlcExposure.Read()", id))
	}
}

func (FfiConverterMaxDustHtlcExposure) Write(writer io.Writer, value MaxDustHtlcExposure) {
	switch variant_value := value.(type) {
	case MaxDustHtlcExposureFixedLimit:
		writeInt32(writer, 1)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.LimitMsat)
	case MaxDustHtlcExposureFeeRateMultiplier:
		writeInt32(writer, 2)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.Multiplier)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterMaxDustHtlcExposure.Write", value))
	}
}

type FfiDestroyerMaxDustHtlcExposure struct{}

func (_ FfiDestroyerMaxDustHtlcExposure) Destroy(value MaxDustHtlcExposure) {
	value.Destroy()
}

type MigrateStorage uint

const (
	MigrateStorageVss MigrateStorage = 1
)

type FfiConverterMigrateStorage struct{}

var FfiConverterMigrateStorageINSTANCE = FfiConverterMigrateStorage{}

func (c FfiConverterMigrateStorage) Lift(rb RustBufferI) MigrateStorage {
	return LiftFromRustBuffer[MigrateStorage](c, rb)
}

func (c FfiConverterMigrateStorage) Lower(value MigrateStorage) C.RustBuffer {
	return LowerIntoRustBuffer[MigrateStorage](c, value)
}
func (FfiConverterMigrateStorage) Read(reader io.Reader) MigrateStorage {
	id := readInt32(reader)
	return MigrateStorage(id)
}

func (FfiConverterMigrateStorage) Write(writer io.Writer, value MigrateStorage) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerMigrateStorage struct{}

func (_ FfiDestroyerMigrateStorage) Destroy(value MigrateStorage) {
}

type Network uint

const (
	NetworkBitcoin Network = 1
	NetworkTestnet Network = 2
	NetworkSignet  Network = 3
	NetworkRegtest Network = 4
)

type FfiConverterNetwork struct{}

var FfiConverterNetworkINSTANCE = FfiConverterNetwork{}

func (c FfiConverterNetwork) Lift(rb RustBufferI) Network {
	return LiftFromRustBuffer[Network](c, rb)
}

func (c FfiConverterNetwork) Lower(value Network) C.RustBuffer {
	return LowerIntoRustBuffer[Network](c, value)
}
func (FfiConverterNetwork) Read(reader io.Reader) Network {
	id := readInt32(reader)
	return Network(id)
}

func (FfiConverterNetwork) Write(writer io.Writer, value Network) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerNetwork struct{}

func (_ FfiDestroyerNetwork) Destroy(value Network) {
}

type NodeError struct {
	err error
}

// Convience method to turn *NodeError into error
// Avoiding treating nil pointer as non nil error interface
func (err *NodeError) AsError() error {
	if err == nil {
		return nil
	} else {
		return err
	}
}

func (err NodeError) Error() string {
	return fmt.Sprintf("NodeError: %s", err.err.Error())
}

func (err NodeError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrNodeErrorAlreadyRunning = fmt.Errorf("NodeErrorAlreadyRunning")
var ErrNodeErrorNotRunning = fmt.Errorf("NodeErrorNotRunning")
var ErrNodeErrorOnchainTxCreationFailed = fmt.Errorf("NodeErrorOnchainTxCreationFailed")
var ErrNodeErrorConnectionFailed = fmt.Errorf("NodeErrorConnectionFailed")
var ErrNodeErrorInvoiceCreationFailed = fmt.Errorf("NodeErrorInvoiceCreationFailed")
var ErrNodeErrorInvoiceRequestCreationFailed = fmt.Errorf("NodeErrorInvoiceRequestCreationFailed")
var ErrNodeErrorOfferCreationFailed = fmt.Errorf("NodeErrorOfferCreationFailed")
var ErrNodeErrorRefundCreationFailed = fmt.Errorf("NodeErrorRefundCreationFailed")
var ErrNodeErrorPaymentSendingFailed = fmt.Errorf("NodeErrorPaymentSendingFailed")
var ErrNodeErrorInvalidCustomTlvs = fmt.Errorf("NodeErrorInvalidCustomTlvs")
var ErrNodeErrorProbeSendingFailed = fmt.Errorf("NodeErrorProbeSendingFailed")
var ErrNodeErrorChannelCreationFailed = fmt.Errorf("NodeErrorChannelCreationFailed")
var ErrNodeErrorChannelClosingFailed = fmt.Errorf("NodeErrorChannelClosingFailed")
var ErrNodeErrorChannelSplicingFailed = fmt.Errorf("NodeErrorChannelSplicingFailed")
var ErrNodeErrorChannelConfigUpdateFailed = fmt.Errorf("NodeErrorChannelConfigUpdateFailed")
var ErrNodeErrorPersistenceFailed = fmt.Errorf("NodeErrorPersistenceFailed")
var ErrNodeErrorFeerateEstimationUpdateFailed = fmt.Errorf("NodeErrorFeerateEstimationUpdateFailed")
var ErrNodeErrorFeerateEstimationUpdateTimeout = fmt.Errorf("NodeErrorFeerateEstimationUpdateTimeout")
var ErrNodeErrorWalletOperationFailed = fmt.Errorf("NodeErrorWalletOperationFailed")
var ErrNodeErrorWalletOperationTimeout = fmt.Errorf("NodeErrorWalletOperationTimeout")
var ErrNodeErrorOnchainTxSigningFailed = fmt.Errorf("NodeErrorOnchainTxSigningFailed")
var ErrNodeErrorTxSyncFailed = fmt.Errorf("NodeErrorTxSyncFailed")
var ErrNodeErrorTxSyncTimeout = fmt.Errorf("NodeErrorTxSyncTimeout")
var ErrNodeErrorGossipUpdateFailed = fmt.Errorf("NodeErrorGossipUpdateFailed")
var ErrNodeErrorGossipUpdateTimeout = fmt.Errorf("NodeErrorGossipUpdateTimeout")
var ErrNodeErrorLiquidityRequestFailed = fmt.Errorf("NodeErrorLiquidityRequestFailed")
var ErrNodeErrorUriParameterParsingFailed = fmt.Errorf("NodeErrorUriParameterParsingFailed")
var ErrNodeErrorInvalidAddress = fmt.Errorf("NodeErrorInvalidAddress")
var ErrNodeErrorInvalidSocketAddress = fmt.Errorf("NodeErrorInvalidSocketAddress")
var ErrNodeErrorInvalidPublicKey = fmt.Errorf("NodeErrorInvalidPublicKey")
var ErrNodeErrorInvalidSecretKey = fmt.Errorf("NodeErrorInvalidSecretKey")
var ErrNodeErrorInvalidOfferId = fmt.Errorf("NodeErrorInvalidOfferId")
var ErrNodeErrorInvalidNodeId = fmt.Errorf("NodeErrorInvalidNodeId")
var ErrNodeErrorInvalidPaymentId = fmt.Errorf("NodeErrorInvalidPaymentId")
var ErrNodeErrorInvalidPaymentHash = fmt.Errorf("NodeErrorInvalidPaymentHash")
var ErrNodeErrorInvalidPaymentPreimage = fmt.Errorf("NodeErrorInvalidPaymentPreimage")
var ErrNodeErrorInvalidPaymentSecret = fmt.Errorf("NodeErrorInvalidPaymentSecret")
var ErrNodeErrorInvalidAmount = fmt.Errorf("NodeErrorInvalidAmount")
var ErrNodeErrorInvalidInvoice = fmt.Errorf("NodeErrorInvalidInvoice")
var ErrNodeErrorInvalidOffer = fmt.Errorf("NodeErrorInvalidOffer")
var ErrNodeErrorInvalidRefund = fmt.Errorf("NodeErrorInvalidRefund")
var ErrNodeErrorInvalidChannelId = fmt.Errorf("NodeErrorInvalidChannelId")
var ErrNodeErrorInvalidNetwork = fmt.Errorf("NodeErrorInvalidNetwork")
var ErrNodeErrorInvalidCustomTlv = fmt.Errorf("NodeErrorInvalidCustomTlv")
var ErrNodeErrorInvalidUri = fmt.Errorf("NodeErrorInvalidUri")
var ErrNodeErrorInvalidQuantity = fmt.Errorf("NodeErrorInvalidQuantity")
var ErrNodeErrorInvalidNodeAlias = fmt.Errorf("NodeErrorInvalidNodeAlias")
var ErrNodeErrorInvalidDateTime = fmt.Errorf("NodeErrorInvalidDateTime")
var ErrNodeErrorInvalidFeeRate = fmt.Errorf("NodeErrorInvalidFeeRate")
var ErrNodeErrorDuplicatePayment = fmt.Errorf("NodeErrorDuplicatePayment")
var ErrNodeErrorUnsupportedCurrency = fmt.Errorf("NodeErrorUnsupportedCurrency")
var ErrNodeErrorInsufficientFunds = fmt.Errorf("NodeErrorInsufficientFunds")
var ErrNodeErrorLiquiditySourceUnavailable = fmt.Errorf("NodeErrorLiquiditySourceUnavailable")
var ErrNodeErrorLiquidityFeeTooHigh = fmt.Errorf("NodeErrorLiquidityFeeTooHigh")
var ErrNodeErrorInvalidBlindedPaths = fmt.Errorf("NodeErrorInvalidBlindedPaths")
var ErrNodeErrorAsyncPaymentServicesDisabled = fmt.Errorf("NodeErrorAsyncPaymentServicesDisabled")

// Variant structs
type NodeErrorAlreadyRunning struct {
	message string
}

func NewNodeErrorAlreadyRunning() *NodeError {
	return &NodeError{err: &NodeErrorAlreadyRunning{}}
}

func (e NodeErrorAlreadyRunning) destroy() {
}

func (err NodeErrorAlreadyRunning) Error() string {
	return fmt.Sprintf("AlreadyRunning: %s", err.message)
}

func (self NodeErrorAlreadyRunning) Is(target error) bool {
	return target == ErrNodeErrorAlreadyRunning
}

type NodeErrorNotRunning struct {
	message string
}

func NewNodeErrorNotRunning() *NodeError {
	return &NodeError{err: &NodeErrorNotRunning{}}
}

func (e NodeErrorNotRunning) destroy() {
}

func (err NodeErrorNotRunning) Error() string {
	return fmt.Sprintf("NotRunning: %s", err.message)
}

func (self NodeErrorNotRunning) Is(target error) bool {
	return target == ErrNodeErrorNotRunning
}

type NodeErrorOnchainTxCreationFailed struct {
	message string
}

func NewNodeErrorOnchainTxCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorOnchainTxCreationFailed{}}
}

func (e NodeErrorOnchainTxCreationFailed) destroy() {
}

func (err NodeErrorOnchainTxCreationFailed) Error() string {
	return fmt.Sprintf("OnchainTxCreationFailed: %s", err.message)
}

func (self NodeErrorOnchainTxCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorOnchainTxCreationFailed
}

type NodeErrorConnectionFailed struct {
	message string
}

func NewNodeErrorConnectionFailed() *NodeError {
	return &NodeError{err: &NodeErrorConnectionFailed{}}
}

func (e NodeErrorConnectionFailed) destroy() {
}

func (err NodeErrorConnectionFailed) Error() string {
	return fmt.Sprintf("ConnectionFailed: %s", err.message)
}

func (self NodeErrorConnectionFailed) Is(target error) bool {
	return target == ErrNodeErrorConnectionFailed
}

type NodeErrorInvoiceCreationFailed struct {
	message string
}

func NewNodeErrorInvoiceCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorInvoiceCreationFailed{}}
}

func (e NodeErrorInvoiceCreationFailed) destroy() {
}

func (err NodeErrorInvoiceCreationFailed) Error() string {
	return fmt.Sprintf("InvoiceCreationFailed: %s", err.message)
}

func (self NodeErrorInvoiceCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorInvoiceCreationFailed
}

type NodeErrorInvoiceRequestCreationFailed struct {
	message string
}

func NewNodeErrorInvoiceRequestCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorInvoiceRequestCreationFailed{}}
}

func (e NodeErrorInvoiceRequestCreationFailed) destroy() {
}

func (err NodeErrorInvoiceRequestCreationFailed) Error() string {
	return fmt.Sprintf("InvoiceRequestCreationFailed: %s", err.message)
}

func (self NodeErrorInvoiceRequestCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorInvoiceRequestCreationFailed
}

type NodeErrorOfferCreationFailed struct {
	message string
}

func NewNodeErrorOfferCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorOfferCreationFailed{}}
}

func (e NodeErrorOfferCreationFailed) destroy() {
}

func (err NodeErrorOfferCreationFailed) Error() string {
	return fmt.Sprintf("OfferCreationFailed: %s", err.message)
}

func (self NodeErrorOfferCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorOfferCreationFailed
}

type NodeErrorRefundCreationFailed struct {
	message string
}

func NewNodeErrorRefundCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorRefundCreationFailed{}}
}

func (e NodeErrorRefundCreationFailed) destroy() {
}

func (err NodeErrorRefundCreationFailed) Error() string {
	return fmt.Sprintf("RefundCreationFailed: %s", err.message)
}

func (self NodeErrorRefundCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorRefundCreationFailed
}

type NodeErrorPaymentSendingFailed struct {
	message string
}

func NewNodeErrorPaymentSendingFailed() *NodeError {
	return &NodeError{err: &NodeErrorPaymentSendingFailed{}}
}

func (e NodeErrorPaymentSendingFailed) destroy() {
}

func (err NodeErrorPaymentSendingFailed) Error() string {
	return fmt.Sprintf("PaymentSendingFailed: %s", err.message)
}

func (self NodeErrorPaymentSendingFailed) Is(target error) bool {
	return target == ErrNodeErrorPaymentSendingFailed
}

type NodeErrorInvalidCustomTlvs struct {
	message string
}

func NewNodeErrorInvalidCustomTlvs() *NodeError {
	return &NodeError{err: &NodeErrorInvalidCustomTlvs{}}
}

func (e NodeErrorInvalidCustomTlvs) destroy() {
}

func (err NodeErrorInvalidCustomTlvs) Error() string {
	return fmt.Sprintf("InvalidCustomTlvs: %s", err.message)
}

func (self NodeErrorInvalidCustomTlvs) Is(target error) bool {
	return target == ErrNodeErrorInvalidCustomTlvs
}

type NodeErrorProbeSendingFailed struct {
	message string
}

func NewNodeErrorProbeSendingFailed() *NodeError {
	return &NodeError{err: &NodeErrorProbeSendingFailed{}}
}

func (e NodeErrorProbeSendingFailed) destroy() {
}

func (err NodeErrorProbeSendingFailed) Error() string {
	return fmt.Sprintf("ProbeSendingFailed: %s", err.message)
}

func (self NodeErrorProbeSendingFailed) Is(target error) bool {
	return target == ErrNodeErrorProbeSendingFailed
}

type NodeErrorChannelCreationFailed struct {
	message string
}

func NewNodeErrorChannelCreationFailed() *NodeError {
	return &NodeError{err: &NodeErrorChannelCreationFailed{}}
}

func (e NodeErrorChannelCreationFailed) destroy() {
}

func (err NodeErrorChannelCreationFailed) Error() string {
	return fmt.Sprintf("ChannelCreationFailed: %s", err.message)
}

func (self NodeErrorChannelCreationFailed) Is(target error) bool {
	return target == ErrNodeErrorChannelCreationFailed
}

type NodeErrorChannelClosingFailed struct {
	message string
}

func NewNodeErrorChannelClosingFailed() *NodeError {
	return &NodeError{err: &NodeErrorChannelClosingFailed{}}
}

func (e NodeErrorChannelClosingFailed) destroy() {
}

func (err NodeErrorChannelClosingFailed) Error() string {
	return fmt.Sprintf("ChannelClosingFailed: %s", err.message)
}

func (self NodeErrorChannelClosingFailed) Is(target error) bool {
	return target == ErrNodeErrorChannelClosingFailed
}

type NodeErrorChannelSplicingFailed struct {
	message string
}

func NewNodeErrorChannelSplicingFailed() *NodeError {
	return &NodeError{err: &NodeErrorChannelSplicingFailed{}}
}

func (e NodeErrorChannelSplicingFailed) destroy() {
}

func (err NodeErrorChannelSplicingFailed) Error() string {
	return fmt.Sprintf("ChannelSplicingFailed: %s", err.message)
}

func (self NodeErrorChannelSplicingFailed) Is(target error) bool {
	return target == ErrNodeErrorChannelSplicingFailed
}

type NodeErrorChannelConfigUpdateFailed struct {
	message string
}

func NewNodeErrorChannelConfigUpdateFailed() *NodeError {
	return &NodeError{err: &NodeErrorChannelConfigUpdateFailed{}}
}

func (e NodeErrorChannelConfigUpdateFailed) destroy() {
}

func (err NodeErrorChannelConfigUpdateFailed) Error() string {
	return fmt.Sprintf("ChannelConfigUpdateFailed: %s", err.message)
}

func (self NodeErrorChannelConfigUpdateFailed) Is(target error) bool {
	return target == ErrNodeErrorChannelConfigUpdateFailed
}

type NodeErrorPersistenceFailed struct {
	message string
}

func NewNodeErrorPersistenceFailed() *NodeError {
	return &NodeError{err: &NodeErrorPersistenceFailed{}}
}

func (e NodeErrorPersistenceFailed) destroy() {
}

func (err NodeErrorPersistenceFailed) Error() string {
	return fmt.Sprintf("PersistenceFailed: %s", err.message)
}

func (self NodeErrorPersistenceFailed) Is(target error) bool {
	return target == ErrNodeErrorPersistenceFailed
}

type NodeErrorFeerateEstimationUpdateFailed struct {
	message string
}

func NewNodeErrorFeerateEstimationUpdateFailed() *NodeError {
	return &NodeError{err: &NodeErrorFeerateEstimationUpdateFailed{}}
}

func (e NodeErrorFeerateEstimationUpdateFailed) destroy() {
}

func (err NodeErrorFeerateEstimationUpdateFailed) Error() string {
	return fmt.Sprintf("FeerateEstimationUpdateFailed: %s", err.message)
}

func (self NodeErrorFeerateEstimationUpdateFailed) Is(target error) bool {
	return target == ErrNodeErrorFeerateEstimationUpdateFailed
}

type NodeErrorFeerateEstimationUpdateTimeout struct {
	message string
}

func NewNodeErrorFeerateEstimationUpdateTimeout() *NodeError {
	return &NodeError{err: &NodeErrorFeerateEstimationUpdateTimeout{}}
}

func (e NodeErrorFeerateEstimationUpdateTimeout) destroy() {
}

func (err NodeErrorFeerateEstimationUpdateTimeout) Error() string {
	return fmt.Sprintf("FeerateEstimationUpdateTimeout: %s", err.message)
}

func (self NodeErrorFeerateEstimationUpdateTimeout) Is(target error) bool {
	return target == ErrNodeErrorFeerateEstimationUpdateTimeout
}

type NodeErrorWalletOperationFailed struct {
	message string
}

func NewNodeErrorWalletOperationFailed() *NodeError {
	return &NodeError{err: &NodeErrorWalletOperationFailed{}}
}

func (e NodeErrorWalletOperationFailed) destroy() {
}

func (err NodeErrorWalletOperationFailed) Error() string {
	return fmt.Sprintf("WalletOperationFailed: %s", err.message)
}

func (self NodeErrorWalletOperationFailed) Is(target error) bool {
	return target == ErrNodeErrorWalletOperationFailed
}

type NodeErrorWalletOperationTimeout struct {
	message string
}

func NewNodeErrorWalletOperationTimeout() *NodeError {
	return &NodeError{err: &NodeErrorWalletOperationTimeout{}}
}

func (e NodeErrorWalletOperationTimeout) destroy() {
}

func (err NodeErrorWalletOperationTimeout) Error() string {
	return fmt.Sprintf("WalletOperationTimeout: %s", err.message)
}

func (self NodeErrorWalletOperationTimeout) Is(target error) bool {
	return target == ErrNodeErrorWalletOperationTimeout
}

type NodeErrorOnchainTxSigningFailed struct {
	message string
}

func NewNodeErrorOnchainTxSigningFailed() *NodeError {
	return &NodeError{err: &NodeErrorOnchainTxSigningFailed{}}
}

func (e NodeErrorOnchainTxSigningFailed) destroy() {
}

func (err NodeErrorOnchainTxSigningFailed) Error() string {
	return fmt.Sprintf("OnchainTxSigningFailed: %s", err.message)
}

func (self NodeErrorOnchainTxSigningFailed) Is(target error) bool {
	return target == ErrNodeErrorOnchainTxSigningFailed
}

type NodeErrorTxSyncFailed struct {
	message string
}

func NewNodeErrorTxSyncFailed() *NodeError {
	return &NodeError{err: &NodeErrorTxSyncFailed{}}
}

func (e NodeErrorTxSyncFailed) destroy() {
}

func (err NodeErrorTxSyncFailed) Error() string {
	return fmt.Sprintf("TxSyncFailed: %s", err.message)
}

func (self NodeErrorTxSyncFailed) Is(target error) bool {
	return target == ErrNodeErrorTxSyncFailed
}

type NodeErrorTxSyncTimeout struct {
	message string
}

func NewNodeErrorTxSyncTimeout() *NodeError {
	return &NodeError{err: &NodeErrorTxSyncTimeout{}}
}

func (e NodeErrorTxSyncTimeout) destroy() {
}

func (err NodeErrorTxSyncTimeout) Error() string {
	return fmt.Sprintf("TxSyncTimeout: %s", err.message)
}

func (self NodeErrorTxSyncTimeout) Is(target error) bool {
	return target == ErrNodeErrorTxSyncTimeout
}

type NodeErrorGossipUpdateFailed struct {
	message string
}

func NewNodeErrorGossipUpdateFailed() *NodeError {
	return &NodeError{err: &NodeErrorGossipUpdateFailed{}}
}

func (e NodeErrorGossipUpdateFailed) destroy() {
}

func (err NodeErrorGossipUpdateFailed) Error() string {
	return fmt.Sprintf("GossipUpdateFailed: %s", err.message)
}

func (self NodeErrorGossipUpdateFailed) Is(target error) bool {
	return target == ErrNodeErrorGossipUpdateFailed
}

type NodeErrorGossipUpdateTimeout struct {
	message string
}

func NewNodeErrorGossipUpdateTimeout() *NodeError {
	return &NodeError{err: &NodeErrorGossipUpdateTimeout{}}
}

func (e NodeErrorGossipUpdateTimeout) destroy() {
}

func (err NodeErrorGossipUpdateTimeout) Error() string {
	return fmt.Sprintf("GossipUpdateTimeout: %s", err.message)
}

func (self NodeErrorGossipUpdateTimeout) Is(target error) bool {
	return target == ErrNodeErrorGossipUpdateTimeout
}

type NodeErrorLiquidityRequestFailed struct {
	message string
}

func NewNodeErrorLiquidityRequestFailed() *NodeError {
	return &NodeError{err: &NodeErrorLiquidityRequestFailed{}}
}

func (e NodeErrorLiquidityRequestFailed) destroy() {
}

func (err NodeErrorLiquidityRequestFailed) Error() string {
	return fmt.Sprintf("LiquidityRequestFailed: %s", err.message)
}

func (self NodeErrorLiquidityRequestFailed) Is(target error) bool {
	return target == ErrNodeErrorLiquidityRequestFailed
}

type NodeErrorUriParameterParsingFailed struct {
	message string
}

func NewNodeErrorUriParameterParsingFailed() *NodeError {
	return &NodeError{err: &NodeErrorUriParameterParsingFailed{}}
}

func (e NodeErrorUriParameterParsingFailed) destroy() {
}

func (err NodeErrorUriParameterParsingFailed) Error() string {
	return fmt.Sprintf("UriParameterParsingFailed: %s", err.message)
}

func (self NodeErrorUriParameterParsingFailed) Is(target error) bool {
	return target == ErrNodeErrorUriParameterParsingFailed
}

type NodeErrorInvalidAddress struct {
	message string
}

func NewNodeErrorInvalidAddress() *NodeError {
	return &NodeError{err: &NodeErrorInvalidAddress{}}
}

func (e NodeErrorInvalidAddress) destroy() {
}

func (err NodeErrorInvalidAddress) Error() string {
	return fmt.Sprintf("InvalidAddress: %s", err.message)
}

func (self NodeErrorInvalidAddress) Is(target error) bool {
	return target == ErrNodeErrorInvalidAddress
}

type NodeErrorInvalidSocketAddress struct {
	message string
}

func NewNodeErrorInvalidSocketAddress() *NodeError {
	return &NodeError{err: &NodeErrorInvalidSocketAddress{}}
}

func (e NodeErrorInvalidSocketAddress) destroy() {
}

func (err NodeErrorInvalidSocketAddress) Error() string {
	return fmt.Sprintf("InvalidSocketAddress: %s", err.message)
}

func (self NodeErrorInvalidSocketAddress) Is(target error) bool {
	return target == ErrNodeErrorInvalidSocketAddress
}

type NodeErrorInvalidPublicKey struct {
	message string
}

func NewNodeErrorInvalidPublicKey() *NodeError {
	return &NodeError{err: &NodeErrorInvalidPublicKey{}}
}

func (e NodeErrorInvalidPublicKey) destroy() {
}

func (err NodeErrorInvalidPublicKey) Error() string {
	return fmt.Sprintf("InvalidPublicKey: %s", err.message)
}

func (self NodeErrorInvalidPublicKey) Is(target error) bool {
	return target == ErrNodeErrorInvalidPublicKey
}

type NodeErrorInvalidSecretKey struct {
	message string
}

func NewNodeErrorInvalidSecretKey() *NodeError {
	return &NodeError{err: &NodeErrorInvalidSecretKey{}}
}

func (e NodeErrorInvalidSecretKey) destroy() {
}

func (err NodeErrorInvalidSecretKey) Error() string {
	return fmt.Sprintf("InvalidSecretKey: %s", err.message)
}

func (self NodeErrorInvalidSecretKey) Is(target error) bool {
	return target == ErrNodeErrorInvalidSecretKey
}

type NodeErrorInvalidOfferId struct {
	message string
}

func NewNodeErrorInvalidOfferId() *NodeError {
	return &NodeError{err: &NodeErrorInvalidOfferId{}}
}

func (e NodeErrorInvalidOfferId) destroy() {
}

func (err NodeErrorInvalidOfferId) Error() string {
	return fmt.Sprintf("InvalidOfferId: %s", err.message)
}

func (self NodeErrorInvalidOfferId) Is(target error) bool {
	return target == ErrNodeErrorInvalidOfferId
}

type NodeErrorInvalidNodeId struct {
	message string
}

func NewNodeErrorInvalidNodeId() *NodeError {
	return &NodeError{err: &NodeErrorInvalidNodeId{}}
}

func (e NodeErrorInvalidNodeId) destroy() {
}

func (err NodeErrorInvalidNodeId) Error() string {
	return fmt.Sprintf("InvalidNodeId: %s", err.message)
}

func (self NodeErrorInvalidNodeId) Is(target error) bool {
	return target == ErrNodeErrorInvalidNodeId
}

type NodeErrorInvalidPaymentId struct {
	message string
}

func NewNodeErrorInvalidPaymentId() *NodeError {
	return &NodeError{err: &NodeErrorInvalidPaymentId{}}
}

func (e NodeErrorInvalidPaymentId) destroy() {
}

func (err NodeErrorInvalidPaymentId) Error() string {
	return fmt.Sprintf("InvalidPaymentId: %s", err.message)
}

func (self NodeErrorInvalidPaymentId) Is(target error) bool {
	return target == ErrNodeErrorInvalidPaymentId
}

type NodeErrorInvalidPaymentHash struct {
	message string
}

func NewNodeErrorInvalidPaymentHash() *NodeError {
	return &NodeError{err: &NodeErrorInvalidPaymentHash{}}
}

func (e NodeErrorInvalidPaymentHash) destroy() {
}

func (err NodeErrorInvalidPaymentHash) Error() string {
	return fmt.Sprintf("InvalidPaymentHash: %s", err.message)
}

func (self NodeErrorInvalidPaymentHash) Is(target error) bool {
	return target == ErrNodeErrorInvalidPaymentHash
}

type NodeErrorInvalidPaymentPreimage struct {
	message string
}

func NewNodeErrorInvalidPaymentPreimage() *NodeError {
	return &NodeError{err: &NodeErrorInvalidPaymentPreimage{}}
}

func (e NodeErrorInvalidPaymentPreimage) destroy() {
}

func (err NodeErrorInvalidPaymentPreimage) Error() string {
	return fmt.Sprintf("InvalidPaymentPreimage: %s", err.message)
}

func (self NodeErrorInvalidPaymentPreimage) Is(target error) bool {
	return target == ErrNodeErrorInvalidPaymentPreimage
}

type NodeErrorInvalidPaymentSecret struct {
	message string
}

func NewNodeErrorInvalidPaymentSecret() *NodeError {
	return &NodeError{err: &NodeErrorInvalidPaymentSecret{}}
}

func (e NodeErrorInvalidPaymentSecret) destroy() {
}

func (err NodeErrorInvalidPaymentSecret) Error() string {
	return fmt.Sprintf("InvalidPaymentSecret: %s", err.message)
}

func (self NodeErrorInvalidPaymentSecret) Is(target error) bool {
	return target == ErrNodeErrorInvalidPaymentSecret
}

type NodeErrorInvalidAmount struct {
	message string
}

func NewNodeErrorInvalidAmount() *NodeError {
	return &NodeError{err: &NodeErrorInvalidAmount{}}
}

func (e NodeErrorInvalidAmount) destroy() {
}

func (err NodeErrorInvalidAmount) Error() string {
	return fmt.Sprintf("InvalidAmount: %s", err.message)
}

func (self NodeErrorInvalidAmount) Is(target error) bool {
	return target == ErrNodeErrorInvalidAmount
}

type NodeErrorInvalidInvoice struct {
	message string
}

func NewNodeErrorInvalidInvoice() *NodeError {
	return &NodeError{err: &NodeErrorInvalidInvoice{}}
}

func (e NodeErrorInvalidInvoice) destroy() {
}

func (err NodeErrorInvalidInvoice) Error() string {
	return fmt.Sprintf("InvalidInvoice: %s", err.message)
}

func (self NodeErrorInvalidInvoice) Is(target error) bool {
	return target == ErrNodeErrorInvalidInvoice
}

type NodeErrorInvalidOffer struct {
	message string
}

func NewNodeErrorInvalidOffer() *NodeError {
	return &NodeError{err: &NodeErrorInvalidOffer{}}
}

func (e NodeErrorInvalidOffer) destroy() {
}

func (err NodeErrorInvalidOffer) Error() string {
	return fmt.Sprintf("InvalidOffer: %s", err.message)
}

func (self NodeErrorInvalidOffer) Is(target error) bool {
	return target == ErrNodeErrorInvalidOffer
}

type NodeErrorInvalidRefund struct {
	message string
}

func NewNodeErrorInvalidRefund() *NodeError {
	return &NodeError{err: &NodeErrorInvalidRefund{}}
}

func (e NodeErrorInvalidRefund) destroy() {
}

func (err NodeErrorInvalidRefund) Error() string {
	return fmt.Sprintf("InvalidRefund: %s", err.message)
}

func (self NodeErrorInvalidRefund) Is(target error) bool {
	return target == ErrNodeErrorInvalidRefund
}

type NodeErrorInvalidChannelId struct {
	message string
}

func NewNodeErrorInvalidChannelId() *NodeError {
	return &NodeError{err: &NodeErrorInvalidChannelId{}}
}

func (e NodeErrorInvalidChannelId) destroy() {
}

func (err NodeErrorInvalidChannelId) Error() string {
	return fmt.Sprintf("InvalidChannelId: %s", err.message)
}

func (self NodeErrorInvalidChannelId) Is(target error) bool {
	return target == ErrNodeErrorInvalidChannelId
}

type NodeErrorInvalidNetwork struct {
	message string
}

func NewNodeErrorInvalidNetwork() *NodeError {
	return &NodeError{err: &NodeErrorInvalidNetwork{}}
}

func (e NodeErrorInvalidNetwork) destroy() {
}

func (err NodeErrorInvalidNetwork) Error() string {
	return fmt.Sprintf("InvalidNetwork: %s", err.message)
}

func (self NodeErrorInvalidNetwork) Is(target error) bool {
	return target == ErrNodeErrorInvalidNetwork
}

type NodeErrorInvalidCustomTlv struct {
	message string
}

func NewNodeErrorInvalidCustomTlv() *NodeError {
	return &NodeError{err: &NodeErrorInvalidCustomTlv{}}
}

func (e NodeErrorInvalidCustomTlv) destroy() {
}

func (err NodeErrorInvalidCustomTlv) Error() string {
	return fmt.Sprintf("InvalidCustomTlv: %s", err.message)
}

func (self NodeErrorInvalidCustomTlv) Is(target error) bool {
	return target == ErrNodeErrorInvalidCustomTlv
}

type NodeErrorInvalidUri struct {
	message string
}

func NewNodeErrorInvalidUri() *NodeError {
	return &NodeError{err: &NodeErrorInvalidUri{}}
}

func (e NodeErrorInvalidUri) destroy() {
}

func (err NodeErrorInvalidUri) Error() string {
	return fmt.Sprintf("InvalidUri: %s", err.message)
}

func (self NodeErrorInvalidUri) Is(target error) bool {
	return target == ErrNodeErrorInvalidUri
}

type NodeErrorInvalidQuantity struct {
	message string
}

func NewNodeErrorInvalidQuantity() *NodeError {
	return &NodeError{err: &NodeErrorInvalidQuantity{}}
}

func (e NodeErrorInvalidQuantity) destroy() {
}

func (err NodeErrorInvalidQuantity) Error() string {
	return fmt.Sprintf("InvalidQuantity: %s", err.message)
}

func (self NodeErrorInvalidQuantity) Is(target error) bool {
	return target == ErrNodeErrorInvalidQuantity
}

type NodeErrorInvalidNodeAlias struct {
	message string
}

func NewNodeErrorInvalidNodeAlias() *NodeError {
	return &NodeError{err: &NodeErrorInvalidNodeAlias{}}
}

func (e NodeErrorInvalidNodeAlias) destroy() {
}

func (err NodeErrorInvalidNodeAlias) Error() string {
	return fmt.Sprintf("InvalidNodeAlias: %s", err.message)
}

func (self NodeErrorInvalidNodeAlias) Is(target error) bool {
	return target == ErrNodeErrorInvalidNodeAlias
}

type NodeErrorInvalidDateTime struct {
	message string
}

func NewNodeErrorInvalidDateTime() *NodeError {
	return &NodeError{err: &NodeErrorInvalidDateTime{}}
}

func (e NodeErrorInvalidDateTime) destroy() {
}

func (err NodeErrorInvalidDateTime) Error() string {
	return fmt.Sprintf("InvalidDateTime: %s", err.message)
}

func (self NodeErrorInvalidDateTime) Is(target error) bool {
	return target == ErrNodeErrorInvalidDateTime
}

type NodeErrorInvalidFeeRate struct {
	message string
}

func NewNodeErrorInvalidFeeRate() *NodeError {
	return &NodeError{err: &NodeErrorInvalidFeeRate{}}
}

func (e NodeErrorInvalidFeeRate) destroy() {
}

func (err NodeErrorInvalidFeeRate) Error() string {
	return fmt.Sprintf("InvalidFeeRate: %s", err.message)
}

func (self NodeErrorInvalidFeeRate) Is(target error) bool {
	return target == ErrNodeErrorInvalidFeeRate
}

type NodeErrorDuplicatePayment struct {
	message string
}

func NewNodeErrorDuplicatePayment() *NodeError {
	return &NodeError{err: &NodeErrorDuplicatePayment{}}
}

func (e NodeErrorDuplicatePayment) destroy() {
}

func (err NodeErrorDuplicatePayment) Error() string {
	return fmt.Sprintf("DuplicatePayment: %s", err.message)
}

func (self NodeErrorDuplicatePayment) Is(target error) bool {
	return target == ErrNodeErrorDuplicatePayment
}

type NodeErrorUnsupportedCurrency struct {
	message string
}

func NewNodeErrorUnsupportedCurrency() *NodeError {
	return &NodeError{err: &NodeErrorUnsupportedCurrency{}}
}

func (e NodeErrorUnsupportedCurrency) destroy() {
}

func (err NodeErrorUnsupportedCurrency) Error() string {
	return fmt.Sprintf("UnsupportedCurrency: %s", err.message)
}

func (self NodeErrorUnsupportedCurrency) Is(target error) bool {
	return target == ErrNodeErrorUnsupportedCurrency
}

type NodeErrorInsufficientFunds struct {
	message string
}

func NewNodeErrorInsufficientFunds() *NodeError {
	return &NodeError{err: &NodeErrorInsufficientFunds{}}
}

func (e NodeErrorInsufficientFunds) destroy() {
}

func (err NodeErrorInsufficientFunds) Error() string {
	return fmt.Sprintf("InsufficientFunds: %s", err.message)
}

func (self NodeErrorInsufficientFunds) Is(target error) bool {
	return target == ErrNodeErrorInsufficientFunds
}

type NodeErrorLiquiditySourceUnavailable struct {
	message string
}

func NewNodeErrorLiquiditySourceUnavailable() *NodeError {
	return &NodeError{err: &NodeErrorLiquiditySourceUnavailable{}}
}

func (e NodeErrorLiquiditySourceUnavailable) destroy() {
}

func (err NodeErrorLiquiditySourceUnavailable) Error() string {
	return fmt.Sprintf("LiquiditySourceUnavailable: %s", err.message)
}

func (self NodeErrorLiquiditySourceUnavailable) Is(target error) bool {
	return target == ErrNodeErrorLiquiditySourceUnavailable
}

type NodeErrorLiquidityFeeTooHigh struct {
	message string
}

func NewNodeErrorLiquidityFeeTooHigh() *NodeError {
	return &NodeError{err: &NodeErrorLiquidityFeeTooHigh{}}
}

func (e NodeErrorLiquidityFeeTooHigh) destroy() {
}

func (err NodeErrorLiquidityFeeTooHigh) Error() string {
	return fmt.Sprintf("LiquidityFeeTooHigh: %s", err.message)
}

func (self NodeErrorLiquidityFeeTooHigh) Is(target error) bool {
	return target == ErrNodeErrorLiquidityFeeTooHigh
}

type NodeErrorInvalidBlindedPaths struct {
	message string
}

func NewNodeErrorInvalidBlindedPaths() *NodeError {
	return &NodeError{err: &NodeErrorInvalidBlindedPaths{}}
}

func (e NodeErrorInvalidBlindedPaths) destroy() {
}

func (err NodeErrorInvalidBlindedPaths) Error() string {
	return fmt.Sprintf("InvalidBlindedPaths: %s", err.message)
}

func (self NodeErrorInvalidBlindedPaths) Is(target error) bool {
	return target == ErrNodeErrorInvalidBlindedPaths
}

type NodeErrorAsyncPaymentServicesDisabled struct {
	message string
}

func NewNodeErrorAsyncPaymentServicesDisabled() *NodeError {
	return &NodeError{err: &NodeErrorAsyncPaymentServicesDisabled{}}
}

func (e NodeErrorAsyncPaymentServicesDisabled) destroy() {
}

func (err NodeErrorAsyncPaymentServicesDisabled) Error() string {
	return fmt.Sprintf("AsyncPaymentServicesDisabled: %s", err.message)
}

func (self NodeErrorAsyncPaymentServicesDisabled) Is(target error) bool {
	return target == ErrNodeErrorAsyncPaymentServicesDisabled
}

type FfiConverterNodeError struct{}

var FfiConverterNodeErrorINSTANCE = FfiConverterNodeError{}

func (c FfiConverterNodeError) Lift(eb RustBufferI) *NodeError {
	return LiftFromRustBuffer[*NodeError](c, eb)
}

func (c FfiConverterNodeError) Lower(value *NodeError) C.RustBuffer {
	return LowerIntoRustBuffer[*NodeError](c, value)
}

func (c FfiConverterNodeError) Read(reader io.Reader) *NodeError {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &NodeError{&NodeErrorAlreadyRunning{message}}
	case 2:
		return &NodeError{&NodeErrorNotRunning{message}}
	case 3:
		return &NodeError{&NodeErrorOnchainTxCreationFailed{message}}
	case 4:
		return &NodeError{&NodeErrorConnectionFailed{message}}
	case 5:
		return &NodeError{&NodeErrorInvoiceCreationFailed{message}}
	case 6:
		return &NodeError{&NodeErrorInvoiceRequestCreationFailed{message}}
	case 7:
		return &NodeError{&NodeErrorOfferCreationFailed{message}}
	case 8:
		return &NodeError{&NodeErrorRefundCreationFailed{message}}
	case 9:
		return &NodeError{&NodeErrorPaymentSendingFailed{message}}
	case 10:
		return &NodeError{&NodeErrorInvalidCustomTlvs{message}}
	case 11:
		return &NodeError{&NodeErrorProbeSendingFailed{message}}
	case 12:
		return &NodeError{&NodeErrorChannelCreationFailed{message}}
	case 13:
		return &NodeError{&NodeErrorChannelClosingFailed{message}}
	case 14:
		return &NodeError{&NodeErrorChannelSplicingFailed{message}}
	case 15:
		return &NodeError{&NodeErrorChannelConfigUpdateFailed{message}}
	case 16:
		return &NodeError{&NodeErrorPersistenceFailed{message}}
	case 17:
		return &NodeError{&NodeErrorFeerateEstimationUpdateFailed{message}}
	case 18:
		return &NodeError{&NodeErrorFeerateEstimationUpdateTimeout{message}}
	case 19:
		return &NodeError{&NodeErrorWalletOperationFailed{message}}
	case 20:
		return &NodeError{&NodeErrorWalletOperationTimeout{message}}
	case 21:
		return &NodeError{&NodeErrorOnchainTxSigningFailed{message}}
	case 22:
		return &NodeError{&NodeErrorTxSyncFailed{message}}
	case 23:
		return &NodeError{&NodeErrorTxSyncTimeout{message}}
	case 24:
		return &NodeError{&NodeErrorGossipUpdateFailed{message}}
	case 25:
		return &NodeError{&NodeErrorGossipUpdateTimeout{message}}
	case 26:
		return &NodeError{&NodeErrorLiquidityRequestFailed{message}}
	case 27:
		return &NodeError{&NodeErrorUriParameterParsingFailed{message}}
	case 28:
		return &NodeError{&NodeErrorInvalidAddress{message}}
	case 29:
		return &NodeError{&NodeErrorInvalidSocketAddress{message}}
	case 30:
		return &NodeError{&NodeErrorInvalidPublicKey{message}}
	case 31:
		return &NodeError{&NodeErrorInvalidSecretKey{message}}
	case 32:
		return &NodeError{&NodeErrorInvalidOfferId{message}}
	case 33:
		return &NodeError{&NodeErrorInvalidNodeId{message}}
	case 34:
		return &NodeError{&NodeErrorInvalidPaymentId{message}}
	case 35:
		return &NodeError{&NodeErrorInvalidPaymentHash{message}}
	case 36:
		return &NodeError{&NodeErrorInvalidPaymentPreimage{message}}
	case 37:
		return &NodeError{&NodeErrorInvalidPaymentSecret{message}}
	case 38:
		return &NodeError{&NodeErrorInvalidAmount{message}}
	case 39:
		return &NodeError{&NodeErrorInvalidInvoice{message}}
	case 40:
		return &NodeError{&NodeErrorInvalidOffer{message}}
	case 41:
		return &NodeError{&NodeErrorInvalidRefund{message}}
	case 42:
		return &NodeError{&NodeErrorInvalidChannelId{message}}
	case 43:
		return &NodeError{&NodeErrorInvalidNetwork{message}}
	case 44:
		return &NodeError{&NodeErrorInvalidCustomTlv{message}}
	case 45:
		return &NodeError{&NodeErrorInvalidUri{message}}
	case 46:
		return &NodeError{&NodeErrorInvalidQuantity{message}}
	case 47:
		return &NodeError{&NodeErrorInvalidNodeAlias{message}}
	case 48:
		return &NodeError{&NodeErrorInvalidDateTime{message}}
	case 49:
		return &NodeError{&NodeErrorInvalidFeeRate{message}}
	case 50:
		return &NodeError{&NodeErrorDuplicatePayment{message}}
	case 51:
		return &NodeError{&NodeErrorUnsupportedCurrency{message}}
	case 52:
		return &NodeError{&NodeErrorInsufficientFunds{message}}
	case 53:
		return &NodeError{&NodeErrorLiquiditySourceUnavailable{message}}
	case 54:
		return &NodeError{&NodeErrorLiquidityFeeTooHigh{message}}
	case 55:
		return &NodeError{&NodeErrorInvalidBlindedPaths{message}}
	case 56:
		return &NodeError{&NodeErrorAsyncPaymentServicesDisabled{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterNodeError.Read()", errorID))
	}

}

func (c FfiConverterNodeError) Write(writer io.Writer, value *NodeError) {
	switch variantValue := value.err.(type) {
	case *NodeErrorAlreadyRunning:
		writeInt32(writer, 1)
	case *NodeErrorNotRunning:
		writeInt32(writer, 2)
	case *NodeErrorOnchainTxCreationFailed:
		writeInt32(writer, 3)
	case *NodeErrorConnectionFailed:
		writeInt32(writer, 4)
	case *NodeErrorInvoiceCreationFailed:
		writeInt32(writer, 5)
	case *NodeErrorInvoiceRequestCreationFailed:
		writeInt32(writer, 6)
	case *NodeErrorOfferCreationFailed:
		writeInt32(writer, 7)
	case *NodeErrorRefundCreationFailed:
		writeInt32(writer, 8)
	case *NodeErrorPaymentSendingFailed:
		writeInt32(writer, 9)
	case *NodeErrorInvalidCustomTlvs:
		writeInt32(writer, 10)
	case *NodeErrorProbeSendingFailed:
		writeInt32(writer, 11)
	case *NodeErrorChannelCreationFailed:
		writeInt32(writer, 12)
	case *NodeErrorChannelClosingFailed:
		writeInt32(writer, 13)
	case *NodeErrorChannelSplicingFailed:
		writeInt32(writer, 14)
	case *NodeErrorChannelConfigUpdateFailed:
		writeInt32(writer, 15)
	case *NodeErrorPersistenceFailed:
		writeInt32(writer, 16)
	case *NodeErrorFeerateEstimationUpdateFailed:
		writeInt32(writer, 17)
	case *NodeErrorFeerateEstimationUpdateTimeout:
		writeInt32(writer, 18)
	case *NodeErrorWalletOperationFailed:
		writeInt32(writer, 19)
	case *NodeErrorWalletOperationTimeout:
		writeInt32(writer, 20)
	case *NodeErrorOnchainTxSigningFailed:
		writeInt32(writer, 21)
	case *NodeErrorTxSyncFailed:
		writeInt32(writer, 22)
	case *NodeErrorTxSyncTimeout:
		writeInt32(writer, 23)
	case *NodeErrorGossipUpdateFailed:
		writeInt32(writer, 24)
	case *NodeErrorGossipUpdateTimeout:
		writeInt32(writer, 25)
	case *NodeErrorLiquidityRequestFailed:
		writeInt32(writer, 26)
	case *NodeErrorUriParameterParsingFailed:
		writeInt32(writer, 27)
	case *NodeErrorInvalidAddress:
		writeInt32(writer, 28)
	case *NodeErrorInvalidSocketAddress:
		writeInt32(writer, 29)
	case *NodeErrorInvalidPublicKey:
		writeInt32(writer, 30)
	case *NodeErrorInvalidSecretKey:
		writeInt32(writer, 31)
	case *NodeErrorInvalidOfferId:
		writeInt32(writer, 32)
	case *NodeErrorInvalidNodeId:
		writeInt32(writer, 33)
	case *NodeErrorInvalidPaymentId:
		writeInt32(writer, 34)
	case *NodeErrorInvalidPaymentHash:
		writeInt32(writer, 35)
	case *NodeErrorInvalidPaymentPreimage:
		writeInt32(writer, 36)
	case *NodeErrorInvalidPaymentSecret:
		writeInt32(writer, 37)
	case *NodeErrorInvalidAmount:
		writeInt32(writer, 38)
	case *NodeErrorInvalidInvoice:
		writeInt32(writer, 39)
	case *NodeErrorInvalidOffer:
		writeInt32(writer, 40)
	case *NodeErrorInvalidRefund:
		writeInt32(writer, 41)
	case *NodeErrorInvalidChannelId:
		writeInt32(writer, 42)
	case *NodeErrorInvalidNetwork:
		writeInt32(writer, 43)
	case *NodeErrorInvalidCustomTlv:
		writeInt32(writer, 44)
	case *NodeErrorInvalidUri:
		writeInt32(writer, 45)
	case *NodeErrorInvalidQuantity:
		writeInt32(writer, 46)
	case *NodeErrorInvalidNodeAlias:
		writeInt32(writer, 47)
	case *NodeErrorInvalidDateTime:
		writeInt32(writer, 48)
	case *NodeErrorInvalidFeeRate:
		writeInt32(writer, 49)
	case *NodeErrorDuplicatePayment:
		writeInt32(writer, 50)
	case *NodeErrorUnsupportedCurrency:
		writeInt32(writer, 51)
	case *NodeErrorInsufficientFunds:
		writeInt32(writer, 52)
	case *NodeErrorLiquiditySourceUnavailable:
		writeInt32(writer, 53)
	case *NodeErrorLiquidityFeeTooHigh:
		writeInt32(writer, 54)
	case *NodeErrorInvalidBlindedPaths:
		writeInt32(writer, 55)
	case *NodeErrorAsyncPaymentServicesDisabled:
		writeInt32(writer, 56)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterNodeError.Write", value))
	}
}

type FfiDestroyerNodeError struct{}

func (_ FfiDestroyerNodeError) Destroy(value *NodeError) {
	switch variantValue := value.err.(type) {
	case NodeErrorAlreadyRunning:
		variantValue.destroy()
	case NodeErrorNotRunning:
		variantValue.destroy()
	case NodeErrorOnchainTxCreationFailed:
		variantValue.destroy()
	case NodeErrorConnectionFailed:
		variantValue.destroy()
	case NodeErrorInvoiceCreationFailed:
		variantValue.destroy()
	case NodeErrorInvoiceRequestCreationFailed:
		variantValue.destroy()
	case NodeErrorOfferCreationFailed:
		variantValue.destroy()
	case NodeErrorRefundCreationFailed:
		variantValue.destroy()
	case NodeErrorPaymentSendingFailed:
		variantValue.destroy()
	case NodeErrorInvalidCustomTlvs:
		variantValue.destroy()
	case NodeErrorProbeSendingFailed:
		variantValue.destroy()
	case NodeErrorChannelCreationFailed:
		variantValue.destroy()
	case NodeErrorChannelClosingFailed:
		variantValue.destroy()
	case NodeErrorChannelSplicingFailed:
		variantValue.destroy()
	case NodeErrorChannelConfigUpdateFailed:
		variantValue.destroy()
	case NodeErrorPersistenceFailed:
		variantValue.destroy()
	case NodeErrorFeerateEstimationUpdateFailed:
		variantValue.destroy()
	case NodeErrorFeerateEstimationUpdateTimeout:
		variantValue.destroy()
	case NodeErrorWalletOperationFailed:
		variantValue.destroy()
	case NodeErrorWalletOperationTimeout:
		variantValue.destroy()
	case NodeErrorOnchainTxSigningFailed:
		variantValue.destroy()
	case NodeErrorTxSyncFailed:
		variantValue.destroy()
	case NodeErrorTxSyncTimeout:
		variantValue.destroy()
	case NodeErrorGossipUpdateFailed:
		variantValue.destroy()
	case NodeErrorGossipUpdateTimeout:
		variantValue.destroy()
	case NodeErrorLiquidityRequestFailed:
		variantValue.destroy()
	case NodeErrorUriParameterParsingFailed:
		variantValue.destroy()
	case NodeErrorInvalidAddress:
		variantValue.destroy()
	case NodeErrorInvalidSocketAddress:
		variantValue.destroy()
	case NodeErrorInvalidPublicKey:
		variantValue.destroy()
	case NodeErrorInvalidSecretKey:
		variantValue.destroy()
	case NodeErrorInvalidOfferId:
		variantValue.destroy()
	case NodeErrorInvalidNodeId:
		variantValue.destroy()
	case NodeErrorInvalidPaymentId:
		variantValue.destroy()
	case NodeErrorInvalidPaymentHash:
		variantValue.destroy()
	case NodeErrorInvalidPaymentPreimage:
		variantValue.destroy()
	case NodeErrorInvalidPaymentSecret:
		variantValue.destroy()
	case NodeErrorInvalidAmount:
		variantValue.destroy()
	case NodeErrorInvalidInvoice:
		variantValue.destroy()
	case NodeErrorInvalidOffer:
		variantValue.destroy()
	case NodeErrorInvalidRefund:
		variantValue.destroy()
	case NodeErrorInvalidChannelId:
		variantValue.destroy()
	case NodeErrorInvalidNetwork:
		variantValue.destroy()
	case NodeErrorInvalidCustomTlv:
		variantValue.destroy()
	case NodeErrorInvalidUri:
		variantValue.destroy()
	case NodeErrorInvalidQuantity:
		variantValue.destroy()
	case NodeErrorInvalidNodeAlias:
		variantValue.destroy()
	case NodeErrorInvalidDateTime:
		variantValue.destroy()
	case NodeErrorInvalidFeeRate:
		variantValue.destroy()
	case NodeErrorDuplicatePayment:
		variantValue.destroy()
	case NodeErrorUnsupportedCurrency:
		variantValue.destroy()
	case NodeErrorInsufficientFunds:
		variantValue.destroy()
	case NodeErrorLiquiditySourceUnavailable:
		variantValue.destroy()
	case NodeErrorLiquidityFeeTooHigh:
		variantValue.destroy()
	case NodeErrorInvalidBlindedPaths:
		variantValue.destroy()
	case NodeErrorAsyncPaymentServicesDisabled:
		variantValue.destroy()
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiDestroyerNodeError.Destroy", value))
	}
}

type OfferAmount interface {
	Destroy()
}
type OfferAmountBitcoin struct {
	AmountMsats uint64
}

func (e OfferAmountBitcoin) Destroy() {
	FfiDestroyerUint64{}.Destroy(e.AmountMsats)
}

type OfferAmountCurrency struct {
	Iso4217Code string
	Amount      uint64
}

func (e OfferAmountCurrency) Destroy() {
	FfiDestroyerString{}.Destroy(e.Iso4217Code)
	FfiDestroyerUint64{}.Destroy(e.Amount)
}

type FfiConverterOfferAmount struct{}

var FfiConverterOfferAmountINSTANCE = FfiConverterOfferAmount{}

func (c FfiConverterOfferAmount) Lift(rb RustBufferI) OfferAmount {
	return LiftFromRustBuffer[OfferAmount](c, rb)
}

func (c FfiConverterOfferAmount) Lower(value OfferAmount) C.RustBuffer {
	return LowerIntoRustBuffer[OfferAmount](c, value)
}
func (FfiConverterOfferAmount) Read(reader io.Reader) OfferAmount {
	id := readInt32(reader)
	switch id {
	case 1:
		return OfferAmountBitcoin{
			FfiConverterUint64INSTANCE.Read(reader),
		}
	case 2:
		return OfferAmountCurrency{
			FfiConverterStringINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterOfferAmount.Read()", id))
	}
}

func (FfiConverterOfferAmount) Write(writer io.Writer, value OfferAmount) {
	switch variant_value := value.(type) {
	case OfferAmountBitcoin:
		writeInt32(writer, 1)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountMsats)
	case OfferAmountCurrency:
		writeInt32(writer, 2)
		FfiConverterStringINSTANCE.Write(writer, variant_value.Iso4217Code)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.Amount)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterOfferAmount.Write", value))
	}
}

type FfiDestroyerOfferAmount struct{}

func (_ FfiDestroyerOfferAmount) Destroy(value OfferAmount) {
	value.Destroy()
}

type PaymentDirection uint

const (
	PaymentDirectionInbound  PaymentDirection = 1
	PaymentDirectionOutbound PaymentDirection = 2
)

type FfiConverterPaymentDirection struct{}

var FfiConverterPaymentDirectionINSTANCE = FfiConverterPaymentDirection{}

func (c FfiConverterPaymentDirection) Lift(rb RustBufferI) PaymentDirection {
	return LiftFromRustBuffer[PaymentDirection](c, rb)
}

func (c FfiConverterPaymentDirection) Lower(value PaymentDirection) C.RustBuffer {
	return LowerIntoRustBuffer[PaymentDirection](c, value)
}
func (FfiConverterPaymentDirection) Read(reader io.Reader) PaymentDirection {
	id := readInt32(reader)
	return PaymentDirection(id)
}

func (FfiConverterPaymentDirection) Write(writer io.Writer, value PaymentDirection) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerPaymentDirection struct{}

func (_ FfiDestroyerPaymentDirection) Destroy(value PaymentDirection) {
}

type PaymentFailureReason uint

const (
	PaymentFailureReasonRecipientRejected         PaymentFailureReason = 1
	PaymentFailureReasonUserAbandoned             PaymentFailureReason = 2
	PaymentFailureReasonRetriesExhausted          PaymentFailureReason = 3
	PaymentFailureReasonPaymentExpired            PaymentFailureReason = 4
	PaymentFailureReasonRouteNotFound             PaymentFailureReason = 5
	PaymentFailureReasonUnexpectedError           PaymentFailureReason = 6
	PaymentFailureReasonUnknownRequiredFeatures   PaymentFailureReason = 7
	PaymentFailureReasonInvoiceRequestExpired     PaymentFailureReason = 8
	PaymentFailureReasonInvoiceRequestRejected    PaymentFailureReason = 9
	PaymentFailureReasonBlindedPathCreationFailed PaymentFailureReason = 10
)

type FfiConverterPaymentFailureReason struct{}

var FfiConverterPaymentFailureReasonINSTANCE = FfiConverterPaymentFailureReason{}

func (c FfiConverterPaymentFailureReason) Lift(rb RustBufferI) PaymentFailureReason {
	return LiftFromRustBuffer[PaymentFailureReason](c, rb)
}

func (c FfiConverterPaymentFailureReason) Lower(value PaymentFailureReason) C.RustBuffer {
	return LowerIntoRustBuffer[PaymentFailureReason](c, value)
}
func (FfiConverterPaymentFailureReason) Read(reader io.Reader) PaymentFailureReason {
	id := readInt32(reader)
	return PaymentFailureReason(id)
}

func (FfiConverterPaymentFailureReason) Write(writer io.Writer, value PaymentFailureReason) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerPaymentFailureReason struct{}

func (_ FfiDestroyerPaymentFailureReason) Destroy(value PaymentFailureReason) {
}

type PaymentKind interface {
	Destroy()
}
type PaymentKindOnchain struct {
	Txid   Txid
	Status ConfirmationStatus
}

func (e PaymentKindOnchain) Destroy() {
	FfiDestroyerTypeTxid{}.Destroy(e.Txid)
	FfiDestroyerConfirmationStatus{}.Destroy(e.Status)
}

type PaymentKindBolt11 struct {
	Hash          PaymentHash
	Preimage      *PaymentPreimage
	Secret        *PaymentSecret
	Bolt11Invoice *string
}

func (e PaymentKindBolt11) Destroy() {
	FfiDestroyerTypePaymentHash{}.Destroy(e.Hash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.Preimage)
	FfiDestroyerOptionalTypePaymentSecret{}.Destroy(e.Secret)
	FfiDestroyerOptionalString{}.Destroy(e.Bolt11Invoice)
}

type PaymentKindBolt11Jit struct {
	Hash                       PaymentHash
	Preimage                   *PaymentPreimage
	Secret                     *PaymentSecret
	CounterpartySkimmedFeeMsat *uint64
	LspFeeLimits               LspFeeLimits
}

func (e PaymentKindBolt11Jit) Destroy() {
	FfiDestroyerTypePaymentHash{}.Destroy(e.Hash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.Preimage)
	FfiDestroyerOptionalTypePaymentSecret{}.Destroy(e.Secret)
	FfiDestroyerOptionalUint64{}.Destroy(e.CounterpartySkimmedFeeMsat)
	FfiDestroyerLspFeeLimits{}.Destroy(e.LspFeeLimits)
}

type PaymentKindBolt12Offer struct {
	Hash      *PaymentHash
	Preimage  *PaymentPreimage
	Secret    *PaymentSecret
	OfferId   OfferId
	PayerNote *UntrustedString
	Quantity  *uint64
}

func (e PaymentKindBolt12Offer) Destroy() {
	FfiDestroyerOptionalTypePaymentHash{}.Destroy(e.Hash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.Preimage)
	FfiDestroyerOptionalTypePaymentSecret{}.Destroy(e.Secret)
	FfiDestroyerTypeOfferId{}.Destroy(e.OfferId)
	FfiDestroyerOptionalTypeUntrustedString{}.Destroy(e.PayerNote)
	FfiDestroyerOptionalUint64{}.Destroy(e.Quantity)
}

type PaymentKindBolt12Refund struct {
	Hash      *PaymentHash
	Preimage  *PaymentPreimage
	Secret    *PaymentSecret
	PayerNote *UntrustedString
	Quantity  *uint64
}

func (e PaymentKindBolt12Refund) Destroy() {
	FfiDestroyerOptionalTypePaymentHash{}.Destroy(e.Hash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.Preimage)
	FfiDestroyerOptionalTypePaymentSecret{}.Destroy(e.Secret)
	FfiDestroyerOptionalTypeUntrustedString{}.Destroy(e.PayerNote)
	FfiDestroyerOptionalUint64{}.Destroy(e.Quantity)
}

type PaymentKindSpontaneous struct {
	Hash       PaymentHash
	Preimage   *PaymentPreimage
	CustomTlvs []TlvEntry
}

func (e PaymentKindSpontaneous) Destroy() {
	FfiDestroyerTypePaymentHash{}.Destroy(e.Hash)
	FfiDestroyerOptionalTypePaymentPreimage{}.Destroy(e.Preimage)
	FfiDestroyerSequenceTlvEntry{}.Destroy(e.CustomTlvs)
}

type FfiConverterPaymentKind struct{}

var FfiConverterPaymentKindINSTANCE = FfiConverterPaymentKind{}

func (c FfiConverterPaymentKind) Lift(rb RustBufferI) PaymentKind {
	return LiftFromRustBuffer[PaymentKind](c, rb)
}

func (c FfiConverterPaymentKind) Lower(value PaymentKind) C.RustBuffer {
	return LowerIntoRustBuffer[PaymentKind](c, value)
}
func (FfiConverterPaymentKind) Read(reader io.Reader) PaymentKind {
	id := readInt32(reader)
	switch id {
	case 1:
		return PaymentKindOnchain{
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterConfirmationStatusINSTANCE.Read(reader),
		}
	case 2:
		return PaymentKindBolt11{
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentSecretINSTANCE.Read(reader),
			FfiConverterOptionalStringINSTANCE.Read(reader),
		}
	case 3:
		return PaymentKindBolt11Jit{
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentSecretINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
			FfiConverterLspFeeLimitsINSTANCE.Read(reader),
		}
	case 4:
		return PaymentKindBolt12Offer{
			FfiConverterOptionalTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentSecretINSTANCE.Read(reader),
			FfiConverterTypeOfferIdINSTANCE.Read(reader),
			FfiConverterOptionalTypeUntrustedStringINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
		}
	case 5:
		return PaymentKindBolt12Refund{
			FfiConverterOptionalTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentSecretINSTANCE.Read(reader),
			FfiConverterOptionalTypeUntrustedStringINSTANCE.Read(reader),
			FfiConverterOptionalUint64INSTANCE.Read(reader),
		}
	case 6:
		return PaymentKindSpontaneous{
			FfiConverterTypePaymentHashINSTANCE.Read(reader),
			FfiConverterOptionalTypePaymentPreimageINSTANCE.Read(reader),
			FfiConverterSequenceTlvEntryINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterPaymentKind.Read()", id))
	}
}

func (FfiConverterPaymentKind) Write(writer io.Writer, value PaymentKind) {
	switch variant_value := value.(type) {
	case PaymentKindOnchain:
		writeInt32(writer, 1)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.Txid)
		FfiConverterConfirmationStatusINSTANCE.Write(writer, variant_value.Status)
	case PaymentKindBolt11:
		writeInt32(writer, 2)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.Hash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.Preimage)
		FfiConverterOptionalTypePaymentSecretINSTANCE.Write(writer, variant_value.Secret)
		FfiConverterOptionalStringINSTANCE.Write(writer, variant_value.Bolt11Invoice)
	case PaymentKindBolt11Jit:
		writeInt32(writer, 3)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.Hash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.Preimage)
		FfiConverterOptionalTypePaymentSecretINSTANCE.Write(writer, variant_value.Secret)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.CounterpartySkimmedFeeMsat)
		FfiConverterLspFeeLimitsINSTANCE.Write(writer, variant_value.LspFeeLimits)
	case PaymentKindBolt12Offer:
		writeInt32(writer, 4)
		FfiConverterOptionalTypePaymentHashINSTANCE.Write(writer, variant_value.Hash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.Preimage)
		FfiConverterOptionalTypePaymentSecretINSTANCE.Write(writer, variant_value.Secret)
		FfiConverterTypeOfferIdINSTANCE.Write(writer, variant_value.OfferId)
		FfiConverterOptionalTypeUntrustedStringINSTANCE.Write(writer, variant_value.PayerNote)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.Quantity)
	case PaymentKindBolt12Refund:
		writeInt32(writer, 5)
		FfiConverterOptionalTypePaymentHashINSTANCE.Write(writer, variant_value.Hash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.Preimage)
		FfiConverterOptionalTypePaymentSecretINSTANCE.Write(writer, variant_value.Secret)
		FfiConverterOptionalTypeUntrustedStringINSTANCE.Write(writer, variant_value.PayerNote)
		FfiConverterOptionalUint64INSTANCE.Write(writer, variant_value.Quantity)
	case PaymentKindSpontaneous:
		writeInt32(writer, 6)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, variant_value.Hash)
		FfiConverterOptionalTypePaymentPreimageINSTANCE.Write(writer, variant_value.Preimage)
		FfiConverterSequenceTlvEntryINSTANCE.Write(writer, variant_value.CustomTlvs)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterPaymentKind.Write", value))
	}
}

type FfiDestroyerPaymentKind struct{}

func (_ FfiDestroyerPaymentKind) Destroy(value PaymentKind) {
	value.Destroy()
}

type PaymentStatus uint

const (
	PaymentStatusPending   PaymentStatus = 1
	PaymentStatusSucceeded PaymentStatus = 2
	PaymentStatusFailed    PaymentStatus = 3
)

type FfiConverterPaymentStatus struct{}

var FfiConverterPaymentStatusINSTANCE = FfiConverterPaymentStatus{}

func (c FfiConverterPaymentStatus) Lift(rb RustBufferI) PaymentStatus {
	return LiftFromRustBuffer[PaymentStatus](c, rb)
}

func (c FfiConverterPaymentStatus) Lower(value PaymentStatus) C.RustBuffer {
	return LowerIntoRustBuffer[PaymentStatus](c, value)
}
func (FfiConverterPaymentStatus) Read(reader io.Reader) PaymentStatus {
	id := readInt32(reader)
	return PaymentStatus(id)
}

func (FfiConverterPaymentStatus) Write(writer io.Writer, value PaymentStatus) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerPaymentStatus struct{}

func (_ FfiDestroyerPaymentStatus) Destroy(value PaymentStatus) {
}

type PendingSweepBalance interface {
	Destroy()
}
type PendingSweepBalancePendingBroadcast struct {
	ChannelId          *ChannelId
	AmountSatoshis     uint64
	CounterpartyNodeId *PublicKey
	FundingTxId        *Txid
	FundingTxIndex     *uint16
}

func (e PendingSweepBalancePendingBroadcast) Destroy() {
	FfiDestroyerOptionalTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerOptionalUint16{}.Destroy(e.FundingTxIndex)
}

type PendingSweepBalanceBroadcastAwaitingConfirmation struct {
	ChannelId             *ChannelId
	LatestBroadcastHeight uint32
	LatestSpendingTxid    Txid
	AmountSatoshis        uint64
	CounterpartyNodeId    *PublicKey
	FundingTxId           *Txid
	FundingTxIndex        *uint16
}

func (e PendingSweepBalanceBroadcastAwaitingConfirmation) Destroy() {
	FfiDestroyerOptionalTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerUint32{}.Destroy(e.LatestBroadcastHeight)
	FfiDestroyerTypeTxid{}.Destroy(e.LatestSpendingTxid)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerOptionalUint16{}.Destroy(e.FundingTxIndex)
}

type PendingSweepBalanceAwaitingThresholdConfirmations struct {
	ChannelId          *ChannelId
	LatestSpendingTxid Txid
	ConfirmationHash   BlockHash
	ConfirmationHeight uint32
	AmountSatoshis     uint64
	CounterpartyNodeId *PublicKey
	FundingTxId        *Txid
	FundingTxIndex     *uint16
}

func (e PendingSweepBalanceAwaitingThresholdConfirmations) Destroy() {
	FfiDestroyerOptionalTypeChannelId{}.Destroy(e.ChannelId)
	FfiDestroyerTypeTxid{}.Destroy(e.LatestSpendingTxid)
	FfiDestroyerTypeBlockHash{}.Destroy(e.ConfirmationHash)
	FfiDestroyerUint32{}.Destroy(e.ConfirmationHeight)
	FfiDestroyerUint64{}.Destroy(e.AmountSatoshis)
	FfiDestroyerOptionalTypePublicKey{}.Destroy(e.CounterpartyNodeId)
	FfiDestroyerOptionalTypeTxid{}.Destroy(e.FundingTxId)
	FfiDestroyerOptionalUint16{}.Destroy(e.FundingTxIndex)
}

type FfiConverterPendingSweepBalance struct{}

var FfiConverterPendingSweepBalanceINSTANCE = FfiConverterPendingSweepBalance{}

func (c FfiConverterPendingSweepBalance) Lift(rb RustBufferI) PendingSweepBalance {
	return LiftFromRustBuffer[PendingSweepBalance](c, rb)
}

func (c FfiConverterPendingSweepBalance) Lower(value PendingSweepBalance) C.RustBuffer {
	return LowerIntoRustBuffer[PendingSweepBalance](c, value)
}
func (FfiConverterPendingSweepBalance) Read(reader io.Reader) PendingSweepBalance {
	id := readInt32(reader)
	switch id {
	case 1:
		return PendingSweepBalancePendingBroadcast{
			FfiConverterOptionalTypeChannelIdINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalTypeTxidINSTANCE.Read(reader),
			FfiConverterOptionalUint16INSTANCE.Read(reader),
		}
	case 2:
		return PendingSweepBalanceBroadcastAwaitingConfirmation{
			FfiConverterOptionalTypeChannelIdINSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalTypeTxidINSTANCE.Read(reader),
			FfiConverterOptionalUint16INSTANCE.Read(reader),
		}
	case 3:
		return PendingSweepBalanceAwaitingThresholdConfirmations{
			FfiConverterOptionalTypeChannelIdINSTANCE.Read(reader),
			FfiConverterTypeTxidINSTANCE.Read(reader),
			FfiConverterTypeBlockHashINSTANCE.Read(reader),
			FfiConverterUint32INSTANCE.Read(reader),
			FfiConverterUint64INSTANCE.Read(reader),
			FfiConverterOptionalTypePublicKeyINSTANCE.Read(reader),
			FfiConverterOptionalTypeTxidINSTANCE.Read(reader),
			FfiConverterOptionalUint16INSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterPendingSweepBalance.Read()", id))
	}
}

func (FfiConverterPendingSweepBalance) Write(writer io.Writer, value PendingSweepBalance) {
	switch variant_value := value.(type) {
	case PendingSweepBalancePendingBroadcast:
		writeInt32(writer, 1)
		FfiConverterOptionalTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterOptionalUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
	case PendingSweepBalanceBroadcastAwaitingConfirmation:
		writeInt32(writer, 2)
		FfiConverterOptionalTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.LatestBroadcastHeight)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.LatestSpendingTxid)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterOptionalUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
	case PendingSweepBalanceAwaitingThresholdConfirmations:
		writeInt32(writer, 3)
		FfiConverterOptionalTypeChannelIdINSTANCE.Write(writer, variant_value.ChannelId)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.LatestSpendingTxid)
		FfiConverterTypeBlockHashINSTANCE.Write(writer, variant_value.ConfirmationHash)
		FfiConverterUint32INSTANCE.Write(writer, variant_value.ConfirmationHeight)
		FfiConverterUint64INSTANCE.Write(writer, variant_value.AmountSatoshis)
		FfiConverterOptionalTypePublicKeyINSTANCE.Write(writer, variant_value.CounterpartyNodeId)
		FfiConverterOptionalTypeTxidINSTANCE.Write(writer, variant_value.FundingTxId)
		FfiConverterOptionalUint16INSTANCE.Write(writer, variant_value.FundingTxIndex)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterPendingSweepBalance.Write", value))
	}
}

type FfiDestroyerPendingSweepBalance struct{}

func (_ FfiDestroyerPendingSweepBalance) Destroy(value PendingSweepBalance) {
	value.Destroy()
}

type QrPaymentResult interface {
	Destroy()
}
type QrPaymentResultOnchain struct {
	Txid Txid
}

func (e QrPaymentResultOnchain) Destroy() {
	FfiDestroyerTypeTxid{}.Destroy(e.Txid)
}

type QrPaymentResultBolt11 struct {
	PaymentId PaymentId
}

func (e QrPaymentResultBolt11) Destroy() {
	FfiDestroyerTypePaymentId{}.Destroy(e.PaymentId)
}

type QrPaymentResultBolt12 struct {
	PaymentId PaymentId
}

func (e QrPaymentResultBolt12) Destroy() {
	FfiDestroyerTypePaymentId{}.Destroy(e.PaymentId)
}

type FfiConverterQrPaymentResult struct{}

var FfiConverterQrPaymentResultINSTANCE = FfiConverterQrPaymentResult{}

func (c FfiConverterQrPaymentResult) Lift(rb RustBufferI) QrPaymentResult {
	return LiftFromRustBuffer[QrPaymentResult](c, rb)
}

func (c FfiConverterQrPaymentResult) Lower(value QrPaymentResult) C.RustBuffer {
	return LowerIntoRustBuffer[QrPaymentResult](c, value)
}
func (FfiConverterQrPaymentResult) Read(reader io.Reader) QrPaymentResult {
	id := readInt32(reader)
	switch id {
	case 1:
		return QrPaymentResultOnchain{
			FfiConverterTypeTxidINSTANCE.Read(reader),
		}
	case 2:
		return QrPaymentResultBolt11{
			FfiConverterTypePaymentIdINSTANCE.Read(reader),
		}
	case 3:
		return QrPaymentResultBolt12{
			FfiConverterTypePaymentIdINSTANCE.Read(reader),
		}
	default:
		panic(fmt.Sprintf("invalid enum value %v in FfiConverterQrPaymentResult.Read()", id))
	}
}

func (FfiConverterQrPaymentResult) Write(writer io.Writer, value QrPaymentResult) {
	switch variant_value := value.(type) {
	case QrPaymentResultOnchain:
		writeInt32(writer, 1)
		FfiConverterTypeTxidINSTANCE.Write(writer, variant_value.Txid)
	case QrPaymentResultBolt11:
		writeInt32(writer, 2)
		FfiConverterTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
	case QrPaymentResultBolt12:
		writeInt32(writer, 3)
		FfiConverterTypePaymentIdINSTANCE.Write(writer, variant_value.PaymentId)
	default:
		_ = variant_value
		panic(fmt.Sprintf("invalid enum value `%v` in FfiConverterQrPaymentResult.Write", value))
	}
}

type FfiDestroyerQrPaymentResult struct{}

func (_ FfiDestroyerQrPaymentResult) Destroy(value QrPaymentResult) {
	value.Destroy()
}

type ResetState uint

const (
	ResetStateNodeMetrics  ResetState = 1
	ResetStateScorer       ResetState = 2
	ResetStateNetworkGraph ResetState = 3
	ResetStateAll          ResetState = 4
)

type FfiConverterResetState struct{}

var FfiConverterResetStateINSTANCE = FfiConverterResetState{}

func (c FfiConverterResetState) Lift(rb RustBufferI) ResetState {
	return LiftFromRustBuffer[ResetState](c, rb)
}

func (c FfiConverterResetState) Lower(value ResetState) C.RustBuffer {
	return LowerIntoRustBuffer[ResetState](c, value)
}
func (FfiConverterResetState) Read(reader io.Reader) ResetState {
	id := readInt32(reader)
	return ResetState(id)
}

func (FfiConverterResetState) Write(writer io.Writer, value ResetState) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerResetState struct{}

func (_ FfiDestroyerResetState) Destroy(value ResetState) {
}

type VssHeaderProviderError struct {
	err error
}

// Convience method to turn *VssHeaderProviderError into error
// Avoiding treating nil pointer as non nil error interface
func (err *VssHeaderProviderError) AsError() error {
	if err == nil {
		return nil
	} else {
		return err
	}
}

func (err VssHeaderProviderError) Error() string {
	return fmt.Sprintf("VssHeaderProviderError: %s", err.err.Error())
}

func (err VssHeaderProviderError) Unwrap() error {
	return err.err
}

// Err* are used for checking error type with `errors.Is`
var ErrVssHeaderProviderErrorInvalidData = fmt.Errorf("VssHeaderProviderErrorInvalidData")
var ErrVssHeaderProviderErrorRequestError = fmt.Errorf("VssHeaderProviderErrorRequestError")
var ErrVssHeaderProviderErrorAuthorizationError = fmt.Errorf("VssHeaderProviderErrorAuthorizationError")
var ErrVssHeaderProviderErrorInternalError = fmt.Errorf("VssHeaderProviderErrorInternalError")

// Variant structs
type VssHeaderProviderErrorInvalidData struct {
	message string
}

func NewVssHeaderProviderErrorInvalidData() *VssHeaderProviderError {
	return &VssHeaderProviderError{err: &VssHeaderProviderErrorInvalidData{}}
}

func (e VssHeaderProviderErrorInvalidData) destroy() {
}

func (err VssHeaderProviderErrorInvalidData) Error() string {
	return fmt.Sprintf("InvalidData: %s", err.message)
}

func (self VssHeaderProviderErrorInvalidData) Is(target error) bool {
	return target == ErrVssHeaderProviderErrorInvalidData
}

type VssHeaderProviderErrorRequestError struct {
	message string
}

func NewVssHeaderProviderErrorRequestError() *VssHeaderProviderError {
	return &VssHeaderProviderError{err: &VssHeaderProviderErrorRequestError{}}
}

func (e VssHeaderProviderErrorRequestError) destroy() {
}

func (err VssHeaderProviderErrorRequestError) Error() string {
	return fmt.Sprintf("RequestError: %s", err.message)
}

func (self VssHeaderProviderErrorRequestError) Is(target error) bool {
	return target == ErrVssHeaderProviderErrorRequestError
}

type VssHeaderProviderErrorAuthorizationError struct {
	message string
}

func NewVssHeaderProviderErrorAuthorizationError() *VssHeaderProviderError {
	return &VssHeaderProviderError{err: &VssHeaderProviderErrorAuthorizationError{}}
}

func (e VssHeaderProviderErrorAuthorizationError) destroy() {
}

func (err VssHeaderProviderErrorAuthorizationError) Error() string {
	return fmt.Sprintf("AuthorizationError: %s", err.message)
}

func (self VssHeaderProviderErrorAuthorizationError) Is(target error) bool {
	return target == ErrVssHeaderProviderErrorAuthorizationError
}

type VssHeaderProviderErrorInternalError struct {
	message string
}

func NewVssHeaderProviderErrorInternalError() *VssHeaderProviderError {
	return &VssHeaderProviderError{err: &VssHeaderProviderErrorInternalError{}}
}

func (e VssHeaderProviderErrorInternalError) destroy() {
}

func (err VssHeaderProviderErrorInternalError) Error() string {
	return fmt.Sprintf("InternalError: %s", err.message)
}

func (self VssHeaderProviderErrorInternalError) Is(target error) bool {
	return target == ErrVssHeaderProviderErrorInternalError
}

type FfiConverterVssHeaderProviderError struct{}

var FfiConverterVssHeaderProviderErrorINSTANCE = FfiConverterVssHeaderProviderError{}

func (c FfiConverterVssHeaderProviderError) Lift(eb RustBufferI) *VssHeaderProviderError {
	return LiftFromRustBuffer[*VssHeaderProviderError](c, eb)
}

func (c FfiConverterVssHeaderProviderError) Lower(value *VssHeaderProviderError) C.RustBuffer {
	return LowerIntoRustBuffer[*VssHeaderProviderError](c, value)
}

func (c FfiConverterVssHeaderProviderError) Read(reader io.Reader) *VssHeaderProviderError {
	errorID := readUint32(reader)

	message := FfiConverterStringINSTANCE.Read(reader)
	switch errorID {
	case 1:
		return &VssHeaderProviderError{&VssHeaderProviderErrorInvalidData{message}}
	case 2:
		return &VssHeaderProviderError{&VssHeaderProviderErrorRequestError{message}}
	case 3:
		return &VssHeaderProviderError{&VssHeaderProviderErrorAuthorizationError{message}}
	case 4:
		return &VssHeaderProviderError{&VssHeaderProviderErrorInternalError{message}}
	default:
		panic(fmt.Sprintf("Unknown error code %d in FfiConverterVssHeaderProviderError.Read()", errorID))
	}

}

func (c FfiConverterVssHeaderProviderError) Write(writer io.Writer, value *VssHeaderProviderError) {
	switch variantValue := value.err.(type) {
	case *VssHeaderProviderErrorInvalidData:
		writeInt32(writer, 1)
	case *VssHeaderProviderErrorRequestError:
		writeInt32(writer, 2)
	case *VssHeaderProviderErrorAuthorizationError:
		writeInt32(writer, 3)
	case *VssHeaderProviderErrorInternalError:
		writeInt32(writer, 4)
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiConverterVssHeaderProviderError.Write", value))
	}
}

type FfiDestroyerVssHeaderProviderError struct{}

func (_ FfiDestroyerVssHeaderProviderError) Destroy(value *VssHeaderProviderError) {
	switch variantValue := value.err.(type) {
	case VssHeaderProviderErrorInvalidData:
		variantValue.destroy()
	case VssHeaderProviderErrorRequestError:
		variantValue.destroy()
	case VssHeaderProviderErrorAuthorizationError:
		variantValue.destroy()
	case VssHeaderProviderErrorInternalError:
		variantValue.destroy()
	default:
		_ = variantValue
		panic(fmt.Sprintf("invalid error value `%v` in FfiDestroyerVssHeaderProviderError.Destroy", value))
	}
}

type WordCount uint

const (
	WordCountWords12 WordCount = 1
	WordCountWords15 WordCount = 2
	WordCountWords18 WordCount = 3
	WordCountWords21 WordCount = 4
	WordCountWords24 WordCount = 5
)

type FfiConverterWordCount struct{}

var FfiConverterWordCountINSTANCE = FfiConverterWordCount{}

func (c FfiConverterWordCount) Lift(rb RustBufferI) WordCount {
	return LiftFromRustBuffer[WordCount](c, rb)
}

func (c FfiConverterWordCount) Lower(value WordCount) C.RustBuffer {
	return LowerIntoRustBuffer[WordCount](c, value)
}
func (FfiConverterWordCount) Read(reader io.Reader) WordCount {
	id := readInt32(reader)
	return WordCount(id)
}

func (FfiConverterWordCount) Write(writer io.Writer, value WordCount) {
	writeInt32(writer, int32(value))
}

type FfiDestroyerWordCount struct{}

func (_ FfiDestroyerWordCount) Destroy(value WordCount) {
}

type FfiConverterOptionalUint16 struct{}

var FfiConverterOptionalUint16INSTANCE = FfiConverterOptionalUint16{}

func (c FfiConverterOptionalUint16) Lift(rb RustBufferI) *uint16 {
	return LiftFromRustBuffer[*uint16](c, rb)
}

func (_ FfiConverterOptionalUint16) Read(reader io.Reader) *uint16 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint16INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint16) Lower(value *uint16) C.RustBuffer {
	return LowerIntoRustBuffer[*uint16](c, value)
}

func (_ FfiConverterOptionalUint16) Write(writer io.Writer, value *uint16) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint16INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint16 struct{}

func (_ FfiDestroyerOptionalUint16) Destroy(value *uint16) {
	if value != nil {
		FfiDestroyerUint16{}.Destroy(*value)
	}
}

type FfiConverterOptionalUint32 struct{}

var FfiConverterOptionalUint32INSTANCE = FfiConverterOptionalUint32{}

func (c FfiConverterOptionalUint32) Lift(rb RustBufferI) *uint32 {
	return LiftFromRustBuffer[*uint32](c, rb)
}

func (_ FfiConverterOptionalUint32) Read(reader io.Reader) *uint32 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint32INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint32) Lower(value *uint32) C.RustBuffer {
	return LowerIntoRustBuffer[*uint32](c, value)
}

func (_ FfiConverterOptionalUint32) Write(writer io.Writer, value *uint32) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint32INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint32 struct{}

func (_ FfiDestroyerOptionalUint32) Destroy(value *uint32) {
	if value != nil {
		FfiDestroyerUint32{}.Destroy(*value)
	}
}

type FfiConverterOptionalUint64 struct{}

var FfiConverterOptionalUint64INSTANCE = FfiConverterOptionalUint64{}

func (c FfiConverterOptionalUint64) Lift(rb RustBufferI) *uint64 {
	return LiftFromRustBuffer[*uint64](c, rb)
}

func (_ FfiConverterOptionalUint64) Read(reader io.Reader) *uint64 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterUint64INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalUint64) Lower(value *uint64) C.RustBuffer {
	return LowerIntoRustBuffer[*uint64](c, value)
}

func (_ FfiConverterOptionalUint64) Write(writer io.Writer, value *uint64) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterUint64INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalUint64 struct{}

func (_ FfiDestroyerOptionalUint64) Destroy(value *uint64) {
	if value != nil {
		FfiDestroyerUint64{}.Destroy(*value)
	}
}

type FfiConverterOptionalBool struct{}

var FfiConverterOptionalBoolINSTANCE = FfiConverterOptionalBool{}

func (c FfiConverterOptionalBool) Lift(rb RustBufferI) *bool {
	return LiftFromRustBuffer[*bool](c, rb)
}

func (_ FfiConverterOptionalBool) Read(reader io.Reader) *bool {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterBoolINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalBool) Lower(value *bool) C.RustBuffer {
	return LowerIntoRustBuffer[*bool](c, value)
}

func (_ FfiConverterOptionalBool) Write(writer io.Writer, value *bool) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterBoolINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalBool struct{}

func (_ FfiDestroyerOptionalBool) Destroy(value *bool) {
	if value != nil {
		FfiDestroyerBool{}.Destroy(*value)
	}
}

type FfiConverterOptionalString struct{}

var FfiConverterOptionalStringINSTANCE = FfiConverterOptionalString{}

func (c FfiConverterOptionalString) Lift(rb RustBufferI) *string {
	return LiftFromRustBuffer[*string](c, rb)
}

func (_ FfiConverterOptionalString) Read(reader io.Reader) *string {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterStringINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalString) Lower(value *string) C.RustBuffer {
	return LowerIntoRustBuffer[*string](c, value)
}

func (_ FfiConverterOptionalString) Write(writer io.Writer, value *string) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterStringINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalString struct{}

func (_ FfiDestroyerOptionalString) Destroy(value *string) {
	if value != nil {
		FfiDestroyerString{}.Destroy(*value)
	}
}

type FfiConverterOptionalFeeRate struct{}

var FfiConverterOptionalFeeRateINSTANCE = FfiConverterOptionalFeeRate{}

func (c FfiConverterOptionalFeeRate) Lift(rb RustBufferI) **FeeRate {
	return LiftFromRustBuffer[**FeeRate](c, rb)
}

func (_ FfiConverterOptionalFeeRate) Read(reader io.Reader) **FeeRate {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterFeeRateINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalFeeRate) Lower(value **FeeRate) C.RustBuffer {
	return LowerIntoRustBuffer[**FeeRate](c, value)
}

func (_ FfiConverterOptionalFeeRate) Write(writer io.Writer, value **FeeRate) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterFeeRateINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalFeeRate struct{}

func (_ FfiDestroyerOptionalFeeRate) Destroy(value **FeeRate) {
	if value != nil {
		FfiDestroyerFeeRate{}.Destroy(*value)
	}
}

type FfiConverterOptionalAnchorChannelsConfig struct{}

var FfiConverterOptionalAnchorChannelsConfigINSTANCE = FfiConverterOptionalAnchorChannelsConfig{}

func (c FfiConverterOptionalAnchorChannelsConfig) Lift(rb RustBufferI) *AnchorChannelsConfig {
	return LiftFromRustBuffer[*AnchorChannelsConfig](c, rb)
}

func (_ FfiConverterOptionalAnchorChannelsConfig) Read(reader io.Reader) *AnchorChannelsConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterAnchorChannelsConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalAnchorChannelsConfig) Lower(value *AnchorChannelsConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*AnchorChannelsConfig](c, value)
}

func (_ FfiConverterOptionalAnchorChannelsConfig) Write(writer io.Writer, value *AnchorChannelsConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterAnchorChannelsConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalAnchorChannelsConfig struct{}

func (_ FfiDestroyerOptionalAnchorChannelsConfig) Destroy(value *AnchorChannelsConfig) {
	if value != nil {
		FfiDestroyerAnchorChannelsConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalBackgroundSyncConfig struct{}

var FfiConverterOptionalBackgroundSyncConfigINSTANCE = FfiConverterOptionalBackgroundSyncConfig{}

func (c FfiConverterOptionalBackgroundSyncConfig) Lift(rb RustBufferI) *BackgroundSyncConfig {
	return LiftFromRustBuffer[*BackgroundSyncConfig](c, rb)
}

func (_ FfiConverterOptionalBackgroundSyncConfig) Read(reader io.Reader) *BackgroundSyncConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterBackgroundSyncConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalBackgroundSyncConfig) Lower(value *BackgroundSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*BackgroundSyncConfig](c, value)
}

func (_ FfiConverterOptionalBackgroundSyncConfig) Write(writer io.Writer, value *BackgroundSyncConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterBackgroundSyncConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalBackgroundSyncConfig struct{}

func (_ FfiDestroyerOptionalBackgroundSyncConfig) Destroy(value *BackgroundSyncConfig) {
	if value != nil {
		FfiDestroyerBackgroundSyncConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalChannelConfig struct{}

var FfiConverterOptionalChannelConfigINSTANCE = FfiConverterOptionalChannelConfig{}

func (c FfiConverterOptionalChannelConfig) Lift(rb RustBufferI) *ChannelConfig {
	return LiftFromRustBuffer[*ChannelConfig](c, rb)
}

func (_ FfiConverterOptionalChannelConfig) Read(reader io.Reader) *ChannelConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterChannelConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalChannelConfig) Lower(value *ChannelConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*ChannelConfig](c, value)
}

func (_ FfiConverterOptionalChannelConfig) Write(writer io.Writer, value *ChannelConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterChannelConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalChannelConfig struct{}

func (_ FfiDestroyerOptionalChannelConfig) Destroy(value *ChannelConfig) {
	if value != nil {
		FfiDestroyerChannelConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalChannelInfo struct{}

var FfiConverterOptionalChannelInfoINSTANCE = FfiConverterOptionalChannelInfo{}

func (c FfiConverterOptionalChannelInfo) Lift(rb RustBufferI) *ChannelInfo {
	return LiftFromRustBuffer[*ChannelInfo](c, rb)
}

func (_ FfiConverterOptionalChannelInfo) Read(reader io.Reader) *ChannelInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterChannelInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalChannelInfo) Lower(value *ChannelInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*ChannelInfo](c, value)
}

func (_ FfiConverterOptionalChannelInfo) Write(writer io.Writer, value *ChannelInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterChannelInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalChannelInfo struct{}

func (_ FfiDestroyerOptionalChannelInfo) Destroy(value *ChannelInfo) {
	if value != nil {
		FfiDestroyerChannelInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalChannelUpdateInfo struct{}

var FfiConverterOptionalChannelUpdateInfoINSTANCE = FfiConverterOptionalChannelUpdateInfo{}

func (c FfiConverterOptionalChannelUpdateInfo) Lift(rb RustBufferI) *ChannelUpdateInfo {
	return LiftFromRustBuffer[*ChannelUpdateInfo](c, rb)
}

func (_ FfiConverterOptionalChannelUpdateInfo) Read(reader io.Reader) *ChannelUpdateInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterChannelUpdateInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalChannelUpdateInfo) Lower(value *ChannelUpdateInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*ChannelUpdateInfo](c, value)
}

func (_ FfiConverterOptionalChannelUpdateInfo) Write(writer io.Writer, value *ChannelUpdateInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterChannelUpdateInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalChannelUpdateInfo struct{}

func (_ FfiDestroyerOptionalChannelUpdateInfo) Destroy(value *ChannelUpdateInfo) {
	if value != nil {
		FfiDestroyerChannelUpdateInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalElectrumSyncConfig struct{}

var FfiConverterOptionalElectrumSyncConfigINSTANCE = FfiConverterOptionalElectrumSyncConfig{}

func (c FfiConverterOptionalElectrumSyncConfig) Lift(rb RustBufferI) *ElectrumSyncConfig {
	return LiftFromRustBuffer[*ElectrumSyncConfig](c, rb)
}

func (_ FfiConverterOptionalElectrumSyncConfig) Read(reader io.Reader) *ElectrumSyncConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterElectrumSyncConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalElectrumSyncConfig) Lower(value *ElectrumSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*ElectrumSyncConfig](c, value)
}

func (_ FfiConverterOptionalElectrumSyncConfig) Write(writer io.Writer, value *ElectrumSyncConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterElectrumSyncConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalElectrumSyncConfig struct{}

func (_ FfiDestroyerOptionalElectrumSyncConfig) Destroy(value *ElectrumSyncConfig) {
	if value != nil {
		FfiDestroyerElectrumSyncConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalEsploraSyncConfig struct{}

var FfiConverterOptionalEsploraSyncConfigINSTANCE = FfiConverterOptionalEsploraSyncConfig{}

func (c FfiConverterOptionalEsploraSyncConfig) Lift(rb RustBufferI) *EsploraSyncConfig {
	return LiftFromRustBuffer[*EsploraSyncConfig](c, rb)
}

func (_ FfiConverterOptionalEsploraSyncConfig) Read(reader io.Reader) *EsploraSyncConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterEsploraSyncConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalEsploraSyncConfig) Lower(value *EsploraSyncConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*EsploraSyncConfig](c, value)
}

func (_ FfiConverterOptionalEsploraSyncConfig) Write(writer io.Writer, value *EsploraSyncConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterEsploraSyncConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalEsploraSyncConfig struct{}

func (_ FfiDestroyerOptionalEsploraSyncConfig) Destroy(value *EsploraSyncConfig) {
	if value != nil {
		FfiDestroyerEsploraSyncConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalLsps1Bolt11PaymentInfo struct{}

var FfiConverterOptionalLsps1Bolt11PaymentInfoINSTANCE = FfiConverterOptionalLsps1Bolt11PaymentInfo{}

func (c FfiConverterOptionalLsps1Bolt11PaymentInfo) Lift(rb RustBufferI) *Lsps1Bolt11PaymentInfo {
	return LiftFromRustBuffer[*Lsps1Bolt11PaymentInfo](c, rb)
}

func (_ FfiConverterOptionalLsps1Bolt11PaymentInfo) Read(reader io.Reader) *Lsps1Bolt11PaymentInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterLsps1Bolt11PaymentInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalLsps1Bolt11PaymentInfo) Lower(value *Lsps1Bolt11PaymentInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*Lsps1Bolt11PaymentInfo](c, value)
}

func (_ FfiConverterOptionalLsps1Bolt11PaymentInfo) Write(writer io.Writer, value *Lsps1Bolt11PaymentInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterLsps1Bolt11PaymentInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalLsps1Bolt11PaymentInfo struct{}

func (_ FfiDestroyerOptionalLsps1Bolt11PaymentInfo) Destroy(value *Lsps1Bolt11PaymentInfo) {
	if value != nil {
		FfiDestroyerLsps1Bolt11PaymentInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalLsps1ChannelInfo struct{}

var FfiConverterOptionalLsps1ChannelInfoINSTANCE = FfiConverterOptionalLsps1ChannelInfo{}

func (c FfiConverterOptionalLsps1ChannelInfo) Lift(rb RustBufferI) *Lsps1ChannelInfo {
	return LiftFromRustBuffer[*Lsps1ChannelInfo](c, rb)
}

func (_ FfiConverterOptionalLsps1ChannelInfo) Read(reader io.Reader) *Lsps1ChannelInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterLsps1ChannelInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalLsps1ChannelInfo) Lower(value *Lsps1ChannelInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*Lsps1ChannelInfo](c, value)
}

func (_ FfiConverterOptionalLsps1ChannelInfo) Write(writer io.Writer, value *Lsps1ChannelInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterLsps1ChannelInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalLsps1ChannelInfo struct{}

func (_ FfiDestroyerOptionalLsps1ChannelInfo) Destroy(value *Lsps1ChannelInfo) {
	if value != nil {
		FfiDestroyerLsps1ChannelInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalLsps1OnchainPaymentInfo struct{}

var FfiConverterOptionalLsps1OnchainPaymentInfoINSTANCE = FfiConverterOptionalLsps1OnchainPaymentInfo{}

func (c FfiConverterOptionalLsps1OnchainPaymentInfo) Lift(rb RustBufferI) *Lsps1OnchainPaymentInfo {
	return LiftFromRustBuffer[*Lsps1OnchainPaymentInfo](c, rb)
}

func (_ FfiConverterOptionalLsps1OnchainPaymentInfo) Read(reader io.Reader) *Lsps1OnchainPaymentInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterLsps1OnchainPaymentInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalLsps1OnchainPaymentInfo) Lower(value *Lsps1OnchainPaymentInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*Lsps1OnchainPaymentInfo](c, value)
}

func (_ FfiConverterOptionalLsps1OnchainPaymentInfo) Write(writer io.Writer, value *Lsps1OnchainPaymentInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterLsps1OnchainPaymentInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalLsps1OnchainPaymentInfo struct{}

func (_ FfiDestroyerOptionalLsps1OnchainPaymentInfo) Destroy(value *Lsps1OnchainPaymentInfo) {
	if value != nil {
		FfiDestroyerLsps1OnchainPaymentInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalNodeAnnouncementInfo struct{}

var FfiConverterOptionalNodeAnnouncementInfoINSTANCE = FfiConverterOptionalNodeAnnouncementInfo{}

func (c FfiConverterOptionalNodeAnnouncementInfo) Lift(rb RustBufferI) *NodeAnnouncementInfo {
	return LiftFromRustBuffer[*NodeAnnouncementInfo](c, rb)
}

func (_ FfiConverterOptionalNodeAnnouncementInfo) Read(reader io.Reader) *NodeAnnouncementInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterNodeAnnouncementInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalNodeAnnouncementInfo) Lower(value *NodeAnnouncementInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*NodeAnnouncementInfo](c, value)
}

func (_ FfiConverterOptionalNodeAnnouncementInfo) Write(writer io.Writer, value *NodeAnnouncementInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterNodeAnnouncementInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalNodeAnnouncementInfo struct{}

func (_ FfiDestroyerOptionalNodeAnnouncementInfo) Destroy(value *NodeAnnouncementInfo) {
	if value != nil {
		FfiDestroyerNodeAnnouncementInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalNodeInfo struct{}

var FfiConverterOptionalNodeInfoINSTANCE = FfiConverterOptionalNodeInfo{}

func (c FfiConverterOptionalNodeInfo) Lift(rb RustBufferI) *NodeInfo {
	return LiftFromRustBuffer[*NodeInfo](c, rb)
}

func (_ FfiConverterOptionalNodeInfo) Read(reader io.Reader) *NodeInfo {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterNodeInfoINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalNodeInfo) Lower(value *NodeInfo) C.RustBuffer {
	return LowerIntoRustBuffer[*NodeInfo](c, value)
}

func (_ FfiConverterOptionalNodeInfo) Write(writer io.Writer, value *NodeInfo) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterNodeInfoINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalNodeInfo struct{}

func (_ FfiDestroyerOptionalNodeInfo) Destroy(value *NodeInfo) {
	if value != nil {
		FfiDestroyerNodeInfo{}.Destroy(*value)
	}
}

type FfiConverterOptionalOutPoint struct{}

var FfiConverterOptionalOutPointINSTANCE = FfiConverterOptionalOutPoint{}

func (c FfiConverterOptionalOutPoint) Lift(rb RustBufferI) *OutPoint {
	return LiftFromRustBuffer[*OutPoint](c, rb)
}

func (_ FfiConverterOptionalOutPoint) Read(reader io.Reader) *OutPoint {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterOutPointINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalOutPoint) Lower(value *OutPoint) C.RustBuffer {
	return LowerIntoRustBuffer[*OutPoint](c, value)
}

func (_ FfiConverterOptionalOutPoint) Write(writer io.Writer, value *OutPoint) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterOutPointINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalOutPoint struct{}

func (_ FfiDestroyerOptionalOutPoint) Destroy(value *OutPoint) {
	if value != nil {
		FfiDestroyerOutPoint{}.Destroy(*value)
	}
}

type FfiConverterOptionalPaymentDetails struct{}

var FfiConverterOptionalPaymentDetailsINSTANCE = FfiConverterOptionalPaymentDetails{}

func (c FfiConverterOptionalPaymentDetails) Lift(rb RustBufferI) *PaymentDetails {
	return LiftFromRustBuffer[*PaymentDetails](c, rb)
}

func (_ FfiConverterOptionalPaymentDetails) Read(reader io.Reader) *PaymentDetails {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterPaymentDetailsINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalPaymentDetails) Lower(value *PaymentDetails) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentDetails](c, value)
}

func (_ FfiConverterOptionalPaymentDetails) Write(writer io.Writer, value *PaymentDetails) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterPaymentDetailsINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalPaymentDetails struct{}

func (_ FfiDestroyerOptionalPaymentDetails) Destroy(value *PaymentDetails) {
	if value != nil {
		FfiDestroyerPaymentDetails{}.Destroy(*value)
	}
}

type FfiConverterOptionalRouteParametersConfig struct{}

var FfiConverterOptionalRouteParametersConfigINSTANCE = FfiConverterOptionalRouteParametersConfig{}

func (c FfiConverterOptionalRouteParametersConfig) Lift(rb RustBufferI) *RouteParametersConfig {
	return LiftFromRustBuffer[*RouteParametersConfig](c, rb)
}

func (_ FfiConverterOptionalRouteParametersConfig) Read(reader io.Reader) *RouteParametersConfig {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterRouteParametersConfigINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalRouteParametersConfig) Lower(value *RouteParametersConfig) C.RustBuffer {
	return LowerIntoRustBuffer[*RouteParametersConfig](c, value)
}

func (_ FfiConverterOptionalRouteParametersConfig) Write(writer io.Writer, value *RouteParametersConfig) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterRouteParametersConfigINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalRouteParametersConfig struct{}

func (_ FfiDestroyerOptionalRouteParametersConfig) Destroy(value *RouteParametersConfig) {
	if value != nil {
		FfiDestroyerRouteParametersConfig{}.Destroy(*value)
	}
}

type FfiConverterOptionalAsyncPaymentsRole struct{}

var FfiConverterOptionalAsyncPaymentsRoleINSTANCE = FfiConverterOptionalAsyncPaymentsRole{}

func (c FfiConverterOptionalAsyncPaymentsRole) Lift(rb RustBufferI) *AsyncPaymentsRole {
	return LiftFromRustBuffer[*AsyncPaymentsRole](c, rb)
}

func (_ FfiConverterOptionalAsyncPaymentsRole) Read(reader io.Reader) *AsyncPaymentsRole {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterAsyncPaymentsRoleINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalAsyncPaymentsRole) Lower(value *AsyncPaymentsRole) C.RustBuffer {
	return LowerIntoRustBuffer[*AsyncPaymentsRole](c, value)
}

func (_ FfiConverterOptionalAsyncPaymentsRole) Write(writer io.Writer, value *AsyncPaymentsRole) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterAsyncPaymentsRoleINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalAsyncPaymentsRole struct{}

func (_ FfiDestroyerOptionalAsyncPaymentsRole) Destroy(value *AsyncPaymentsRole) {
	if value != nil {
		FfiDestroyerAsyncPaymentsRole{}.Destroy(*value)
	}
}

type FfiConverterOptionalClosureReason struct{}

var FfiConverterOptionalClosureReasonINSTANCE = FfiConverterOptionalClosureReason{}

func (c FfiConverterOptionalClosureReason) Lift(rb RustBufferI) *ClosureReason {
	return LiftFromRustBuffer[*ClosureReason](c, rb)
}

func (_ FfiConverterOptionalClosureReason) Read(reader io.Reader) *ClosureReason {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterClosureReasonINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalClosureReason) Lower(value *ClosureReason) C.RustBuffer {
	return LowerIntoRustBuffer[*ClosureReason](c, value)
}

func (_ FfiConverterOptionalClosureReason) Write(writer io.Writer, value *ClosureReason) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterClosureReasonINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalClosureReason struct{}

func (_ FfiDestroyerOptionalClosureReason) Destroy(value *ClosureReason) {
	if value != nil {
		FfiDestroyerClosureReason{}.Destroy(*value)
	}
}

type FfiConverterOptionalEvent struct{}

var FfiConverterOptionalEventINSTANCE = FfiConverterOptionalEvent{}

func (c FfiConverterOptionalEvent) Lift(rb RustBufferI) *Event {
	return LiftFromRustBuffer[*Event](c, rb)
}

func (_ FfiConverterOptionalEvent) Read(reader io.Reader) *Event {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterEventINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalEvent) Lower(value *Event) C.RustBuffer {
	return LowerIntoRustBuffer[*Event](c, value)
}

func (_ FfiConverterOptionalEvent) Write(writer io.Writer, value *Event) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterEventINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalEvent struct{}

func (_ FfiDestroyerOptionalEvent) Destroy(value *Event) {
	if value != nil {
		FfiDestroyerEvent{}.Destroy(*value)
	}
}

type FfiConverterOptionalLogLevel struct{}

var FfiConverterOptionalLogLevelINSTANCE = FfiConverterOptionalLogLevel{}

func (c FfiConverterOptionalLogLevel) Lift(rb RustBufferI) *LogLevel {
	return LiftFromRustBuffer[*LogLevel](c, rb)
}

func (_ FfiConverterOptionalLogLevel) Read(reader io.Reader) *LogLevel {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterLogLevelINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalLogLevel) Lower(value *LogLevel) C.RustBuffer {
	return LowerIntoRustBuffer[*LogLevel](c, value)
}

func (_ FfiConverterOptionalLogLevel) Write(writer io.Writer, value *LogLevel) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterLogLevelINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalLogLevel struct{}

func (_ FfiDestroyerOptionalLogLevel) Destroy(value *LogLevel) {
	if value != nil {
		FfiDestroyerLogLevel{}.Destroy(*value)
	}
}

type FfiConverterOptionalNetwork struct{}

var FfiConverterOptionalNetworkINSTANCE = FfiConverterOptionalNetwork{}

func (c FfiConverterOptionalNetwork) Lift(rb RustBufferI) *Network {
	return LiftFromRustBuffer[*Network](c, rb)
}

func (_ FfiConverterOptionalNetwork) Read(reader io.Reader) *Network {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterNetworkINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalNetwork) Lower(value *Network) C.RustBuffer {
	return LowerIntoRustBuffer[*Network](c, value)
}

func (_ FfiConverterOptionalNetwork) Write(writer io.Writer, value *Network) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterNetworkINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalNetwork struct{}

func (_ FfiDestroyerOptionalNetwork) Destroy(value *Network) {
	if value != nil {
		FfiDestroyerNetwork{}.Destroy(*value)
	}
}

type FfiConverterOptionalOfferAmount struct{}

var FfiConverterOptionalOfferAmountINSTANCE = FfiConverterOptionalOfferAmount{}

func (c FfiConverterOptionalOfferAmount) Lift(rb RustBufferI) *OfferAmount {
	return LiftFromRustBuffer[*OfferAmount](c, rb)
}

func (_ FfiConverterOptionalOfferAmount) Read(reader io.Reader) *OfferAmount {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterOfferAmountINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalOfferAmount) Lower(value *OfferAmount) C.RustBuffer {
	return LowerIntoRustBuffer[*OfferAmount](c, value)
}

func (_ FfiConverterOptionalOfferAmount) Write(writer io.Writer, value *OfferAmount) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterOfferAmountINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalOfferAmount struct{}

func (_ FfiDestroyerOptionalOfferAmount) Destroy(value *OfferAmount) {
	if value != nil {
		FfiDestroyerOfferAmount{}.Destroy(*value)
	}
}

type FfiConverterOptionalPaymentFailureReason struct{}

var FfiConverterOptionalPaymentFailureReasonINSTANCE = FfiConverterOptionalPaymentFailureReason{}

func (c FfiConverterOptionalPaymentFailureReason) Lift(rb RustBufferI) *PaymentFailureReason {
	return LiftFromRustBuffer[*PaymentFailureReason](c, rb)
}

func (_ FfiConverterOptionalPaymentFailureReason) Read(reader io.Reader) *PaymentFailureReason {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterPaymentFailureReasonINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalPaymentFailureReason) Lower(value *PaymentFailureReason) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentFailureReason](c, value)
}

func (_ FfiConverterOptionalPaymentFailureReason) Write(writer io.Writer, value *PaymentFailureReason) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterPaymentFailureReasonINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalPaymentFailureReason struct{}

func (_ FfiDestroyerOptionalPaymentFailureReason) Destroy(value *PaymentFailureReason) {
	if value != nil {
		FfiDestroyerPaymentFailureReason{}.Destroy(*value)
	}
}

type FfiConverterOptionalWordCount struct{}

var FfiConverterOptionalWordCountINSTANCE = FfiConverterOptionalWordCount{}

func (c FfiConverterOptionalWordCount) Lift(rb RustBufferI) *WordCount {
	return LiftFromRustBuffer[*WordCount](c, rb)
}

func (_ FfiConverterOptionalWordCount) Read(reader io.Reader) *WordCount {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterWordCountINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalWordCount) Lower(value *WordCount) C.RustBuffer {
	return LowerIntoRustBuffer[*WordCount](c, value)
}

func (_ FfiConverterOptionalWordCount) Write(writer io.Writer, value *WordCount) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterWordCountINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalWordCount struct{}

func (_ FfiDestroyerOptionalWordCount) Destroy(value *WordCount) {
	if value != nil {
		FfiDestroyerWordCount{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceUint8 struct{}

var FfiConverterOptionalSequenceUint8INSTANCE = FfiConverterOptionalSequenceUint8{}

func (c FfiConverterOptionalSequenceUint8) Lift(rb RustBufferI) *[]uint8 {
	return LiftFromRustBuffer[*[]uint8](c, rb)
}

func (_ FfiConverterOptionalSequenceUint8) Read(reader io.Reader) *[]uint8 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceUint8INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceUint8) Lower(value *[]uint8) C.RustBuffer {
	return LowerIntoRustBuffer[*[]uint8](c, value)
}

func (_ FfiConverterOptionalSequenceUint8) Write(writer io.Writer, value *[]uint8) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceUint8INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceUint8 struct{}

func (_ FfiDestroyerOptionalSequenceUint8) Destroy(value *[]uint8) {
	if value != nil {
		FfiDestroyerSequenceUint8{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceSequenceUint8 struct{}

var FfiConverterOptionalSequenceSequenceUint8INSTANCE = FfiConverterOptionalSequenceSequenceUint8{}

func (c FfiConverterOptionalSequenceSequenceUint8) Lift(rb RustBufferI) *[][]uint8 {
	return LiftFromRustBuffer[*[][]uint8](c, rb)
}

func (_ FfiConverterOptionalSequenceSequenceUint8) Read(reader io.Reader) *[][]uint8 {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceSequenceUint8INSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceSequenceUint8) Lower(value *[][]uint8) C.RustBuffer {
	return LowerIntoRustBuffer[*[][]uint8](c, value)
}

func (_ FfiConverterOptionalSequenceSequenceUint8) Write(writer io.Writer, value *[][]uint8) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceSequenceUint8INSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceSequenceUint8 struct{}

func (_ FfiDestroyerOptionalSequenceSequenceUint8) Destroy(value *[][]uint8) {
	if value != nil {
		FfiDestroyerSequenceSequenceUint8{}.Destroy(*value)
	}
}

type FfiConverterOptionalSequenceTypeSocketAddress struct{}

var FfiConverterOptionalSequenceTypeSocketAddressINSTANCE = FfiConverterOptionalSequenceTypeSocketAddress{}

func (c FfiConverterOptionalSequenceTypeSocketAddress) Lift(rb RustBufferI) *[]SocketAddress {
	return LiftFromRustBuffer[*[]SocketAddress](c, rb)
}

func (_ FfiConverterOptionalSequenceTypeSocketAddress) Read(reader io.Reader) *[]SocketAddress {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterSequenceTypeSocketAddressINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalSequenceTypeSocketAddress) Lower(value *[]SocketAddress) C.RustBuffer {
	return LowerIntoRustBuffer[*[]SocketAddress](c, value)
}

func (_ FfiConverterOptionalSequenceTypeSocketAddress) Write(writer io.Writer, value *[]SocketAddress) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterSequenceTypeSocketAddressINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalSequenceTypeSocketAddress struct{}

func (_ FfiDestroyerOptionalSequenceTypeSocketAddress) Destroy(value *[]SocketAddress) {
	if value != nil {
		FfiDestroyerSequenceTypeSocketAddress{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeAddress struct{}

var FfiConverterOptionalTypeAddressINSTANCE = FfiConverterOptionalTypeAddress{}

func (c FfiConverterOptionalTypeAddress) Lift(rb RustBufferI) *Address {
	return LiftFromRustBuffer[*Address](c, rb)
}

func (_ FfiConverterOptionalTypeAddress) Read(reader io.Reader) *Address {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeAddressINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeAddress) Lower(value *Address) C.RustBuffer {
	return LowerIntoRustBuffer[*Address](c, value)
}

func (_ FfiConverterOptionalTypeAddress) Write(writer io.Writer, value *Address) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeAddressINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeAddress struct{}

func (_ FfiDestroyerOptionalTypeAddress) Destroy(value *Address) {
	if value != nil {
		FfiDestroyerTypeAddress{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeChannelId struct{}

var FfiConverterOptionalTypeChannelIdINSTANCE = FfiConverterOptionalTypeChannelId{}

func (c FfiConverterOptionalTypeChannelId) Lift(rb RustBufferI) *ChannelId {
	return LiftFromRustBuffer[*ChannelId](c, rb)
}

func (_ FfiConverterOptionalTypeChannelId) Read(reader io.Reader) *ChannelId {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeChannelIdINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeChannelId) Lower(value *ChannelId) C.RustBuffer {
	return LowerIntoRustBuffer[*ChannelId](c, value)
}

func (_ FfiConverterOptionalTypeChannelId) Write(writer io.Writer, value *ChannelId) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeChannelIdINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeChannelId struct{}

func (_ FfiDestroyerOptionalTypeChannelId) Destroy(value *ChannelId) {
	if value != nil {
		FfiDestroyerTypeChannelId{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeNodeAlias struct{}

var FfiConverterOptionalTypeNodeAliasINSTANCE = FfiConverterOptionalTypeNodeAlias{}

func (c FfiConverterOptionalTypeNodeAlias) Lift(rb RustBufferI) *NodeAlias {
	return LiftFromRustBuffer[*NodeAlias](c, rb)
}

func (_ FfiConverterOptionalTypeNodeAlias) Read(reader io.Reader) *NodeAlias {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeNodeAliasINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeNodeAlias) Lower(value *NodeAlias) C.RustBuffer {
	return LowerIntoRustBuffer[*NodeAlias](c, value)
}

func (_ FfiConverterOptionalTypeNodeAlias) Write(writer io.Writer, value *NodeAlias) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeNodeAliasINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeNodeAlias struct{}

func (_ FfiDestroyerOptionalTypeNodeAlias) Destroy(value *NodeAlias) {
	if value != nil {
		FfiDestroyerTypeNodeAlias{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePaymentHash struct{}

var FfiConverterOptionalTypePaymentHashINSTANCE = FfiConverterOptionalTypePaymentHash{}

func (c FfiConverterOptionalTypePaymentHash) Lift(rb RustBufferI) *PaymentHash {
	return LiftFromRustBuffer[*PaymentHash](c, rb)
}

func (_ FfiConverterOptionalTypePaymentHash) Read(reader io.Reader) *PaymentHash {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePaymentHashINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePaymentHash) Lower(value *PaymentHash) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentHash](c, value)
}

func (_ FfiConverterOptionalTypePaymentHash) Write(writer io.Writer, value *PaymentHash) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePaymentHashINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePaymentHash struct{}

func (_ FfiDestroyerOptionalTypePaymentHash) Destroy(value *PaymentHash) {
	if value != nil {
		FfiDestroyerTypePaymentHash{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePaymentId struct{}

var FfiConverterOptionalTypePaymentIdINSTANCE = FfiConverterOptionalTypePaymentId{}

func (c FfiConverterOptionalTypePaymentId) Lift(rb RustBufferI) *PaymentId {
	return LiftFromRustBuffer[*PaymentId](c, rb)
}

func (_ FfiConverterOptionalTypePaymentId) Read(reader io.Reader) *PaymentId {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePaymentIdINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePaymentId) Lower(value *PaymentId) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentId](c, value)
}

func (_ FfiConverterOptionalTypePaymentId) Write(writer io.Writer, value *PaymentId) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePaymentIdINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePaymentId struct{}

func (_ FfiDestroyerOptionalTypePaymentId) Destroy(value *PaymentId) {
	if value != nil {
		FfiDestroyerTypePaymentId{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePaymentPreimage struct{}

var FfiConverterOptionalTypePaymentPreimageINSTANCE = FfiConverterOptionalTypePaymentPreimage{}

func (c FfiConverterOptionalTypePaymentPreimage) Lift(rb RustBufferI) *PaymentPreimage {
	return LiftFromRustBuffer[*PaymentPreimage](c, rb)
}

func (_ FfiConverterOptionalTypePaymentPreimage) Read(reader io.Reader) *PaymentPreimage {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePaymentPreimageINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePaymentPreimage) Lower(value *PaymentPreimage) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentPreimage](c, value)
}

func (_ FfiConverterOptionalTypePaymentPreimage) Write(writer io.Writer, value *PaymentPreimage) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePaymentPreimageINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePaymentPreimage struct{}

func (_ FfiDestroyerOptionalTypePaymentPreimage) Destroy(value *PaymentPreimage) {
	if value != nil {
		FfiDestroyerTypePaymentPreimage{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePaymentSecret struct{}

var FfiConverterOptionalTypePaymentSecretINSTANCE = FfiConverterOptionalTypePaymentSecret{}

func (c FfiConverterOptionalTypePaymentSecret) Lift(rb RustBufferI) *PaymentSecret {
	return LiftFromRustBuffer[*PaymentSecret](c, rb)
}

func (_ FfiConverterOptionalTypePaymentSecret) Read(reader io.Reader) *PaymentSecret {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePaymentSecretINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePaymentSecret) Lower(value *PaymentSecret) C.RustBuffer {
	return LowerIntoRustBuffer[*PaymentSecret](c, value)
}

func (_ FfiConverterOptionalTypePaymentSecret) Write(writer io.Writer, value *PaymentSecret) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePaymentSecretINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePaymentSecret struct{}

func (_ FfiDestroyerOptionalTypePaymentSecret) Destroy(value *PaymentSecret) {
	if value != nil {
		FfiDestroyerTypePaymentSecret{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypePublicKey struct{}

var FfiConverterOptionalTypePublicKeyINSTANCE = FfiConverterOptionalTypePublicKey{}

func (c FfiConverterOptionalTypePublicKey) Lift(rb RustBufferI) *PublicKey {
	return LiftFromRustBuffer[*PublicKey](c, rb)
}

func (_ FfiConverterOptionalTypePublicKey) Read(reader io.Reader) *PublicKey {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypePublicKeyINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypePublicKey) Lower(value *PublicKey) C.RustBuffer {
	return LowerIntoRustBuffer[*PublicKey](c, value)
}

func (_ FfiConverterOptionalTypePublicKey) Write(writer io.Writer, value *PublicKey) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypePublicKeyINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypePublicKey struct{}

func (_ FfiDestroyerOptionalTypePublicKey) Destroy(value *PublicKey) {
	if value != nil {
		FfiDestroyerTypePublicKey{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeTxid struct{}

var FfiConverterOptionalTypeTxidINSTANCE = FfiConverterOptionalTypeTxid{}

func (c FfiConverterOptionalTypeTxid) Lift(rb RustBufferI) *Txid {
	return LiftFromRustBuffer[*Txid](c, rb)
}

func (_ FfiConverterOptionalTypeTxid) Read(reader io.Reader) *Txid {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeTxidINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeTxid) Lower(value *Txid) C.RustBuffer {
	return LowerIntoRustBuffer[*Txid](c, value)
}

func (_ FfiConverterOptionalTypeTxid) Write(writer io.Writer, value *Txid) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeTxidINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeTxid struct{}

func (_ FfiDestroyerOptionalTypeTxid) Destroy(value *Txid) {
	if value != nil {
		FfiDestroyerTypeTxid{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeUntrustedString struct{}

var FfiConverterOptionalTypeUntrustedStringINSTANCE = FfiConverterOptionalTypeUntrustedString{}

func (c FfiConverterOptionalTypeUntrustedString) Lift(rb RustBufferI) *UntrustedString {
	return LiftFromRustBuffer[*UntrustedString](c, rb)
}

func (_ FfiConverterOptionalTypeUntrustedString) Read(reader io.Reader) *UntrustedString {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeUntrustedStringINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeUntrustedString) Lower(value *UntrustedString) C.RustBuffer {
	return LowerIntoRustBuffer[*UntrustedString](c, value)
}

func (_ FfiConverterOptionalTypeUntrustedString) Write(writer io.Writer, value *UntrustedString) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeUntrustedStringINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeUntrustedString struct{}

func (_ FfiDestroyerOptionalTypeUntrustedString) Destroy(value *UntrustedString) {
	if value != nil {
		FfiDestroyerTypeUntrustedString{}.Destroy(*value)
	}
}

type FfiConverterOptionalTypeUserChannelId struct{}

var FfiConverterOptionalTypeUserChannelIdINSTANCE = FfiConverterOptionalTypeUserChannelId{}

func (c FfiConverterOptionalTypeUserChannelId) Lift(rb RustBufferI) *UserChannelId {
	return LiftFromRustBuffer[*UserChannelId](c, rb)
}

func (_ FfiConverterOptionalTypeUserChannelId) Read(reader io.Reader) *UserChannelId {
	if readInt8(reader) == 0 {
		return nil
	}
	temp := FfiConverterTypeUserChannelIdINSTANCE.Read(reader)
	return &temp
}

func (c FfiConverterOptionalTypeUserChannelId) Lower(value *UserChannelId) C.RustBuffer {
	return LowerIntoRustBuffer[*UserChannelId](c, value)
}

func (_ FfiConverterOptionalTypeUserChannelId) Write(writer io.Writer, value *UserChannelId) {
	if value == nil {
		writeInt8(writer, 0)
	} else {
		writeInt8(writer, 1)
		FfiConverterTypeUserChannelIdINSTANCE.Write(writer, *value)
	}
}

type FfiDestroyerOptionalTypeUserChannelId struct{}

func (_ FfiDestroyerOptionalTypeUserChannelId) Destroy(value *UserChannelId) {
	if value != nil {
		FfiDestroyerTypeUserChannelId{}.Destroy(*value)
	}
}

type FfiConverterSequenceUint8 struct{}

var FfiConverterSequenceUint8INSTANCE = FfiConverterSequenceUint8{}

func (c FfiConverterSequenceUint8) Lift(rb RustBufferI) []uint8 {
	return LiftFromRustBuffer[[]uint8](c, rb)
}

func (c FfiConverterSequenceUint8) Read(reader io.Reader) []uint8 {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]uint8, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterUint8INSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceUint8) Lower(value []uint8) C.RustBuffer {
	return LowerIntoRustBuffer[[]uint8](c, value)
}

func (c FfiConverterSequenceUint8) Write(writer io.Writer, value []uint8) {
	if len(value) > math.MaxInt32 {
		panic("[]uint8 is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterUint8INSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceUint8 struct{}

func (FfiDestroyerSequenceUint8) Destroy(sequence []uint8) {
	for _, value := range sequence {
		FfiDestroyerUint8{}.Destroy(value)
	}
}

type FfiConverterSequenceUint64 struct{}

var FfiConverterSequenceUint64INSTANCE = FfiConverterSequenceUint64{}

func (c FfiConverterSequenceUint64) Lift(rb RustBufferI) []uint64 {
	return LiftFromRustBuffer[[]uint64](c, rb)
}

func (c FfiConverterSequenceUint64) Read(reader io.Reader) []uint64 {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]uint64, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterUint64INSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceUint64) Lower(value []uint64) C.RustBuffer {
	return LowerIntoRustBuffer[[]uint64](c, value)
}

func (c FfiConverterSequenceUint64) Write(writer io.Writer, value []uint64) {
	if len(value) > math.MaxInt32 {
		panic("[]uint64 is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterUint64INSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceUint64 struct{}

func (FfiDestroyerSequenceUint64) Destroy(sequence []uint64) {
	for _, value := range sequence {
		FfiDestroyerUint64{}.Destroy(value)
	}
}

type FfiConverterSequenceChannelDetails struct{}

var FfiConverterSequenceChannelDetailsINSTANCE = FfiConverterSequenceChannelDetails{}

func (c FfiConverterSequenceChannelDetails) Lift(rb RustBufferI) []ChannelDetails {
	return LiftFromRustBuffer[[]ChannelDetails](c, rb)
}

func (c FfiConverterSequenceChannelDetails) Read(reader io.Reader) []ChannelDetails {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]ChannelDetails, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterChannelDetailsINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceChannelDetails) Lower(value []ChannelDetails) C.RustBuffer {
	return LowerIntoRustBuffer[[]ChannelDetails](c, value)
}

func (c FfiConverterSequenceChannelDetails) Write(writer io.Writer, value []ChannelDetails) {
	if len(value) > math.MaxInt32 {
		panic("[]ChannelDetails is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterChannelDetailsINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceChannelDetails struct{}

func (FfiDestroyerSequenceChannelDetails) Destroy(sequence []ChannelDetails) {
	for _, value := range sequence {
		FfiDestroyerChannelDetails{}.Destroy(value)
	}
}

type FfiConverterSequenceChannelMonitorSizeInfo struct{}

var FfiConverterSequenceChannelMonitorSizeInfoINSTANCE = FfiConverterSequenceChannelMonitorSizeInfo{}

func (c FfiConverterSequenceChannelMonitorSizeInfo) Lift(rb RustBufferI) []ChannelMonitorSizeInfo {
	return LiftFromRustBuffer[[]ChannelMonitorSizeInfo](c, rb)
}

func (c FfiConverterSequenceChannelMonitorSizeInfo) Read(reader io.Reader) []ChannelMonitorSizeInfo {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]ChannelMonitorSizeInfo, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterChannelMonitorSizeInfoINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceChannelMonitorSizeInfo) Lower(value []ChannelMonitorSizeInfo) C.RustBuffer {
	return LowerIntoRustBuffer[[]ChannelMonitorSizeInfo](c, value)
}

func (c FfiConverterSequenceChannelMonitorSizeInfo) Write(writer io.Writer, value []ChannelMonitorSizeInfo) {
	if len(value) > math.MaxInt32 {
		panic("[]ChannelMonitorSizeInfo is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterChannelMonitorSizeInfoINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceChannelMonitorSizeInfo struct{}

func (FfiDestroyerSequenceChannelMonitorSizeInfo) Destroy(sequence []ChannelMonitorSizeInfo) {
	for _, value := range sequence {
		FfiDestroyerChannelMonitorSizeInfo{}.Destroy(value)
	}
}

type FfiConverterSequenceCustomTlvRecord struct{}

var FfiConverterSequenceCustomTlvRecordINSTANCE = FfiConverterSequenceCustomTlvRecord{}

func (c FfiConverterSequenceCustomTlvRecord) Lift(rb RustBufferI) []CustomTlvRecord {
	return LiftFromRustBuffer[[]CustomTlvRecord](c, rb)
}

func (c FfiConverterSequenceCustomTlvRecord) Read(reader io.Reader) []CustomTlvRecord {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]CustomTlvRecord, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterCustomTlvRecordINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceCustomTlvRecord) Lower(value []CustomTlvRecord) C.RustBuffer {
	return LowerIntoRustBuffer[[]CustomTlvRecord](c, value)
}

func (c FfiConverterSequenceCustomTlvRecord) Write(writer io.Writer, value []CustomTlvRecord) {
	if len(value) > math.MaxInt32 {
		panic("[]CustomTlvRecord is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterCustomTlvRecordINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceCustomTlvRecord struct{}

func (FfiDestroyerSequenceCustomTlvRecord) Destroy(sequence []CustomTlvRecord) {
	for _, value := range sequence {
		FfiDestroyerCustomTlvRecord{}.Destroy(value)
	}
}

type FfiConverterSequenceKeyValue struct{}

var FfiConverterSequenceKeyValueINSTANCE = FfiConverterSequenceKeyValue{}

func (c FfiConverterSequenceKeyValue) Lift(rb RustBufferI) []KeyValue {
	return LiftFromRustBuffer[[]KeyValue](c, rb)
}

func (c FfiConverterSequenceKeyValue) Read(reader io.Reader) []KeyValue {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]KeyValue, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterKeyValueINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceKeyValue) Lower(value []KeyValue) C.RustBuffer {
	return LowerIntoRustBuffer[[]KeyValue](c, value)
}

func (c FfiConverterSequenceKeyValue) Write(writer io.Writer, value []KeyValue) {
	if len(value) > math.MaxInt32 {
		panic("[]KeyValue is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterKeyValueINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceKeyValue struct{}

func (FfiDestroyerSequenceKeyValue) Destroy(sequence []KeyValue) {
	for _, value := range sequence {
		FfiDestroyerKeyValue{}.Destroy(value)
	}
}

type FfiConverterSequenceLsps2OpeningFeeParams struct{}

var FfiConverterSequenceLsps2OpeningFeeParamsINSTANCE = FfiConverterSequenceLsps2OpeningFeeParams{}

func (c FfiConverterSequenceLsps2OpeningFeeParams) Lift(rb RustBufferI) []Lsps2OpeningFeeParams {
	return LiftFromRustBuffer[[]Lsps2OpeningFeeParams](c, rb)
}

func (c FfiConverterSequenceLsps2OpeningFeeParams) Read(reader io.Reader) []Lsps2OpeningFeeParams {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]Lsps2OpeningFeeParams, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterLsps2OpeningFeeParamsINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceLsps2OpeningFeeParams) Lower(value []Lsps2OpeningFeeParams) C.RustBuffer {
	return LowerIntoRustBuffer[[]Lsps2OpeningFeeParams](c, value)
}

func (c FfiConverterSequenceLsps2OpeningFeeParams) Write(writer io.Writer, value []Lsps2OpeningFeeParams) {
	if len(value) > math.MaxInt32 {
		panic("[]Lsps2OpeningFeeParams is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterLsps2OpeningFeeParamsINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceLsps2OpeningFeeParams struct{}

func (FfiDestroyerSequenceLsps2OpeningFeeParams) Destroy(sequence []Lsps2OpeningFeeParams) {
	for _, value := range sequence {
		FfiDestroyerLsps2OpeningFeeParams{}.Destroy(value)
	}
}

type FfiConverterSequencePaymentDetails struct{}

var FfiConverterSequencePaymentDetailsINSTANCE = FfiConverterSequencePaymentDetails{}

func (c FfiConverterSequencePaymentDetails) Lift(rb RustBufferI) []PaymentDetails {
	return LiftFromRustBuffer[[]PaymentDetails](c, rb)
}

func (c FfiConverterSequencePaymentDetails) Read(reader io.Reader) []PaymentDetails {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]PaymentDetails, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterPaymentDetailsINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequencePaymentDetails) Lower(value []PaymentDetails) C.RustBuffer {
	return LowerIntoRustBuffer[[]PaymentDetails](c, value)
}

func (c FfiConverterSequencePaymentDetails) Write(writer io.Writer, value []PaymentDetails) {
	if len(value) > math.MaxInt32 {
		panic("[]PaymentDetails is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterPaymentDetailsINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequencePaymentDetails struct{}

func (FfiDestroyerSequencePaymentDetails) Destroy(sequence []PaymentDetails) {
	for _, value := range sequence {
		FfiDestroyerPaymentDetails{}.Destroy(value)
	}
}

type FfiConverterSequencePeerDetails struct{}

var FfiConverterSequencePeerDetailsINSTANCE = FfiConverterSequencePeerDetails{}

func (c FfiConverterSequencePeerDetails) Lift(rb RustBufferI) []PeerDetails {
	return LiftFromRustBuffer[[]PeerDetails](c, rb)
}

func (c FfiConverterSequencePeerDetails) Read(reader io.Reader) []PeerDetails {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]PeerDetails, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterPeerDetailsINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequencePeerDetails) Lower(value []PeerDetails) C.RustBuffer {
	return LowerIntoRustBuffer[[]PeerDetails](c, value)
}

func (c FfiConverterSequencePeerDetails) Write(writer io.Writer, value []PeerDetails) {
	if len(value) > math.MaxInt32 {
		panic("[]PeerDetails is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterPeerDetailsINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequencePeerDetails struct{}

func (FfiDestroyerSequencePeerDetails) Destroy(sequence []PeerDetails) {
	for _, value := range sequence {
		FfiDestroyerPeerDetails{}.Destroy(value)
	}
}

type FfiConverterSequenceReceivingChannel struct{}

var FfiConverterSequenceReceivingChannelINSTANCE = FfiConverterSequenceReceivingChannel{}

func (c FfiConverterSequenceReceivingChannel) Lift(rb RustBufferI) []ReceivingChannel {
	return LiftFromRustBuffer[[]ReceivingChannel](c, rb)
}

func (c FfiConverterSequenceReceivingChannel) Read(reader io.Reader) []ReceivingChannel {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]ReceivingChannel, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterReceivingChannelINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceReceivingChannel) Lower(value []ReceivingChannel) C.RustBuffer {
	return LowerIntoRustBuffer[[]ReceivingChannel](c, value)
}

func (c FfiConverterSequenceReceivingChannel) Write(writer io.Writer, value []ReceivingChannel) {
	if len(value) > math.MaxInt32 {
		panic("[]ReceivingChannel is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterReceivingChannelINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceReceivingChannel struct{}

func (FfiDestroyerSequenceReceivingChannel) Destroy(sequence []ReceivingChannel) {
	for _, value := range sequence {
		FfiDestroyerReceivingChannel{}.Destroy(value)
	}
}

type FfiConverterSequenceRouteHintHop struct{}

var FfiConverterSequenceRouteHintHopINSTANCE = FfiConverterSequenceRouteHintHop{}

func (c FfiConverterSequenceRouteHintHop) Lift(rb RustBufferI) []RouteHintHop {
	return LiftFromRustBuffer[[]RouteHintHop](c, rb)
}

func (c FfiConverterSequenceRouteHintHop) Read(reader io.Reader) []RouteHintHop {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]RouteHintHop, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterRouteHintHopINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceRouteHintHop) Lower(value []RouteHintHop) C.RustBuffer {
	return LowerIntoRustBuffer[[]RouteHintHop](c, value)
}

func (c FfiConverterSequenceRouteHintHop) Write(writer io.Writer, value []RouteHintHop) {
	if len(value) > math.MaxInt32 {
		panic("[]RouteHintHop is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterRouteHintHopINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceRouteHintHop struct{}

func (FfiDestroyerSequenceRouteHintHop) Destroy(sequence []RouteHintHop) {
	for _, value := range sequence {
		FfiDestroyerRouteHintHop{}.Destroy(value)
	}
}

type FfiConverterSequenceTlvEntry struct{}

var FfiConverterSequenceTlvEntryINSTANCE = FfiConverterSequenceTlvEntry{}

func (c FfiConverterSequenceTlvEntry) Lift(rb RustBufferI) []TlvEntry {
	return LiftFromRustBuffer[[]TlvEntry](c, rb)
}

func (c FfiConverterSequenceTlvEntry) Read(reader io.Reader) []TlvEntry {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]TlvEntry, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTlvEntryINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTlvEntry) Lower(value []TlvEntry) C.RustBuffer {
	return LowerIntoRustBuffer[[]TlvEntry](c, value)
}

func (c FfiConverterSequenceTlvEntry) Write(writer io.Writer, value []TlvEntry) {
	if len(value) > math.MaxInt32 {
		panic("[]TlvEntry is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTlvEntryINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTlvEntry struct{}

func (FfiDestroyerSequenceTlvEntry) Destroy(sequence []TlvEntry) {
	for _, value := range sequence {
		FfiDestroyerTlvEntry{}.Destroy(value)
	}
}

type FfiConverterSequenceLightningBalance struct{}

var FfiConverterSequenceLightningBalanceINSTANCE = FfiConverterSequenceLightningBalance{}

func (c FfiConverterSequenceLightningBalance) Lift(rb RustBufferI) []LightningBalance {
	return LiftFromRustBuffer[[]LightningBalance](c, rb)
}

func (c FfiConverterSequenceLightningBalance) Read(reader io.Reader) []LightningBalance {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]LightningBalance, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterLightningBalanceINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceLightningBalance) Lower(value []LightningBalance) C.RustBuffer {
	return LowerIntoRustBuffer[[]LightningBalance](c, value)
}

func (c FfiConverterSequenceLightningBalance) Write(writer io.Writer, value []LightningBalance) {
	if len(value) > math.MaxInt32 {
		panic("[]LightningBalance is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterLightningBalanceINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceLightningBalance struct{}

func (FfiDestroyerSequenceLightningBalance) Destroy(sequence []LightningBalance) {
	for _, value := range sequence {
		FfiDestroyerLightningBalance{}.Destroy(value)
	}
}

type FfiConverterSequenceNetwork struct{}

var FfiConverterSequenceNetworkINSTANCE = FfiConverterSequenceNetwork{}

func (c FfiConverterSequenceNetwork) Lift(rb RustBufferI) []Network {
	return LiftFromRustBuffer[[]Network](c, rb)
}

func (c FfiConverterSequenceNetwork) Read(reader io.Reader) []Network {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]Network, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterNetworkINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceNetwork) Lower(value []Network) C.RustBuffer {
	return LowerIntoRustBuffer[[]Network](c, value)
}

func (c FfiConverterSequenceNetwork) Write(writer io.Writer, value []Network) {
	if len(value) > math.MaxInt32 {
		panic("[]Network is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterNetworkINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceNetwork struct{}

func (FfiDestroyerSequenceNetwork) Destroy(sequence []Network) {
	for _, value := range sequence {
		FfiDestroyerNetwork{}.Destroy(value)
	}
}

type FfiConverterSequencePendingSweepBalance struct{}

var FfiConverterSequencePendingSweepBalanceINSTANCE = FfiConverterSequencePendingSweepBalance{}

func (c FfiConverterSequencePendingSweepBalance) Lift(rb RustBufferI) []PendingSweepBalance {
	return LiftFromRustBuffer[[]PendingSweepBalance](c, rb)
}

func (c FfiConverterSequencePendingSweepBalance) Read(reader io.Reader) []PendingSweepBalance {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]PendingSweepBalance, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterPendingSweepBalanceINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequencePendingSweepBalance) Lower(value []PendingSweepBalance) C.RustBuffer {
	return LowerIntoRustBuffer[[]PendingSweepBalance](c, value)
}

func (c FfiConverterSequencePendingSweepBalance) Write(writer io.Writer, value []PendingSweepBalance) {
	if len(value) > math.MaxInt32 {
		panic("[]PendingSweepBalance is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterPendingSweepBalanceINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequencePendingSweepBalance struct{}

func (FfiDestroyerSequencePendingSweepBalance) Destroy(sequence []PendingSweepBalance) {
	for _, value := range sequence {
		FfiDestroyerPendingSweepBalance{}.Destroy(value)
	}
}

type FfiConverterSequenceSequenceUint8 struct{}

var FfiConverterSequenceSequenceUint8INSTANCE = FfiConverterSequenceSequenceUint8{}

func (c FfiConverterSequenceSequenceUint8) Lift(rb RustBufferI) [][]uint8 {
	return LiftFromRustBuffer[[][]uint8](c, rb)
}

func (c FfiConverterSequenceSequenceUint8) Read(reader io.Reader) [][]uint8 {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([][]uint8, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterSequenceUint8INSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceSequenceUint8) Lower(value [][]uint8) C.RustBuffer {
	return LowerIntoRustBuffer[[][]uint8](c, value)
}

func (c FfiConverterSequenceSequenceUint8) Write(writer io.Writer, value [][]uint8) {
	if len(value) > math.MaxInt32 {
		panic("[][]uint8 is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterSequenceUint8INSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceSequenceUint8 struct{}

func (FfiDestroyerSequenceSequenceUint8) Destroy(sequence [][]uint8) {
	for _, value := range sequence {
		FfiDestroyerSequenceUint8{}.Destroy(value)
	}
}

type FfiConverterSequenceSequenceRouteHintHop struct{}

var FfiConverterSequenceSequenceRouteHintHopINSTANCE = FfiConverterSequenceSequenceRouteHintHop{}

func (c FfiConverterSequenceSequenceRouteHintHop) Lift(rb RustBufferI) [][]RouteHintHop {
	return LiftFromRustBuffer[[][]RouteHintHop](c, rb)
}

func (c FfiConverterSequenceSequenceRouteHintHop) Read(reader io.Reader) [][]RouteHintHop {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([][]RouteHintHop, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterSequenceRouteHintHopINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceSequenceRouteHintHop) Lower(value [][]RouteHintHop) C.RustBuffer {
	return LowerIntoRustBuffer[[][]RouteHintHop](c, value)
}

func (c FfiConverterSequenceSequenceRouteHintHop) Write(writer io.Writer, value [][]RouteHintHop) {
	if len(value) > math.MaxInt32 {
		panic("[][]RouteHintHop is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterSequenceRouteHintHopINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceSequenceRouteHintHop struct{}

func (FfiDestroyerSequenceSequenceRouteHintHop) Destroy(sequence [][]RouteHintHop) {
	for _, value := range sequence {
		FfiDestroyerSequenceRouteHintHop{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeAddress struct{}

var FfiConverterSequenceTypeAddressINSTANCE = FfiConverterSequenceTypeAddress{}

func (c FfiConverterSequenceTypeAddress) Lift(rb RustBufferI) []Address {
	return LiftFromRustBuffer[[]Address](c, rb)
}

func (c FfiConverterSequenceTypeAddress) Read(reader io.Reader) []Address {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]Address, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeAddressINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeAddress) Lower(value []Address) C.RustBuffer {
	return LowerIntoRustBuffer[[]Address](c, value)
}

func (c FfiConverterSequenceTypeAddress) Write(writer io.Writer, value []Address) {
	if len(value) > math.MaxInt32 {
		panic("[]Address is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeAddressINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeAddress struct{}

func (FfiDestroyerSequenceTypeAddress) Destroy(sequence []Address) {
	for _, value := range sequence {
		FfiDestroyerTypeAddress{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeNodeId struct{}

var FfiConverterSequenceTypeNodeIdINSTANCE = FfiConverterSequenceTypeNodeId{}

func (c FfiConverterSequenceTypeNodeId) Lift(rb RustBufferI) []NodeId {
	return LiftFromRustBuffer[[]NodeId](c, rb)
}

func (c FfiConverterSequenceTypeNodeId) Read(reader io.Reader) []NodeId {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]NodeId, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeNodeIdINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeNodeId) Lower(value []NodeId) C.RustBuffer {
	return LowerIntoRustBuffer[[]NodeId](c, value)
}

func (c FfiConverterSequenceTypeNodeId) Write(writer io.Writer, value []NodeId) {
	if len(value) > math.MaxInt32 {
		panic("[]NodeId is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeNodeIdINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeNodeId struct{}

func (FfiDestroyerSequenceTypeNodeId) Destroy(sequence []NodeId) {
	for _, value := range sequence {
		FfiDestroyerTypeNodeId{}.Destroy(value)
	}
}

type FfiConverterSequenceTypePublicKey struct{}

var FfiConverterSequenceTypePublicKeyINSTANCE = FfiConverterSequenceTypePublicKey{}

func (c FfiConverterSequenceTypePublicKey) Lift(rb RustBufferI) []PublicKey {
	return LiftFromRustBuffer[[]PublicKey](c, rb)
}

func (c FfiConverterSequenceTypePublicKey) Read(reader io.Reader) []PublicKey {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]PublicKey, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypePublicKeyINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypePublicKey) Lower(value []PublicKey) C.RustBuffer {
	return LowerIntoRustBuffer[[]PublicKey](c, value)
}

func (c FfiConverterSequenceTypePublicKey) Write(writer io.Writer, value []PublicKey) {
	if len(value) > math.MaxInt32 {
		panic("[]PublicKey is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypePublicKeyINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypePublicKey struct{}

func (FfiDestroyerSequenceTypePublicKey) Destroy(sequence []PublicKey) {
	for _, value := range sequence {
		FfiDestroyerTypePublicKey{}.Destroy(value)
	}
}

type FfiConverterSequenceTypeSocketAddress struct{}

var FfiConverterSequenceTypeSocketAddressINSTANCE = FfiConverterSequenceTypeSocketAddress{}

func (c FfiConverterSequenceTypeSocketAddress) Lift(rb RustBufferI) []SocketAddress {
	return LiftFromRustBuffer[[]SocketAddress](c, rb)
}

func (c FfiConverterSequenceTypeSocketAddress) Read(reader io.Reader) []SocketAddress {
	length := readInt32(reader)
	if length == 0 {
		return nil
	}
	result := make([]SocketAddress, 0, length)
	for i := int32(0); i < length; i++ {
		result = append(result, FfiConverterTypeSocketAddressINSTANCE.Read(reader))
	}
	return result
}

func (c FfiConverterSequenceTypeSocketAddress) Lower(value []SocketAddress) C.RustBuffer {
	return LowerIntoRustBuffer[[]SocketAddress](c, value)
}

func (c FfiConverterSequenceTypeSocketAddress) Write(writer io.Writer, value []SocketAddress) {
	if len(value) > math.MaxInt32 {
		panic("[]SocketAddress is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(value)))
	for _, item := range value {
		FfiConverterTypeSocketAddressINSTANCE.Write(writer, item)
	}
}

type FfiDestroyerSequenceTypeSocketAddress struct{}

func (FfiDestroyerSequenceTypeSocketAddress) Destroy(sequence []SocketAddress) {
	for _, value := range sequence {
		FfiDestroyerTypeSocketAddress{}.Destroy(value)
	}
}

type FfiConverterMapStringString struct{}

var FfiConverterMapStringStringINSTANCE = FfiConverterMapStringString{}

func (c FfiConverterMapStringString) Lift(rb RustBufferI) map[string]string {
	return LiftFromRustBuffer[map[string]string](c, rb)
}

func (_ FfiConverterMapStringString) Read(reader io.Reader) map[string]string {
	result := make(map[string]string)
	length := readInt32(reader)
	for i := int32(0); i < length; i++ {
		key := FfiConverterStringINSTANCE.Read(reader)
		value := FfiConverterStringINSTANCE.Read(reader)
		result[key] = value
	}
	return result
}

func (c FfiConverterMapStringString) Lower(value map[string]string) C.RustBuffer {
	return LowerIntoRustBuffer[map[string]string](c, value)
}

func (_ FfiConverterMapStringString) Write(writer io.Writer, mapValue map[string]string) {
	if len(mapValue) > math.MaxInt32 {
		panic("map[string]string is too large to fit into Int32")
	}

	writeInt32(writer, int32(len(mapValue)))
	for key, value := range mapValue {
		FfiConverterStringINSTANCE.Write(writer, key)
		FfiConverterStringINSTANCE.Write(writer, value)
	}
}

type FfiDestroyerMapStringString struct{}

func (_ FfiDestroyerMapStringString) Destroy(mapValue map[string]string) {
	for key, value := range mapValue {
		FfiDestroyerString{}.Destroy(key)
		FfiDestroyerString{}.Destroy(value)
	}
}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type Address = string
type FfiConverterTypeAddress = FfiConverterString
type FfiDestroyerTypeAddress = FfiDestroyerString

var FfiConverterTypeAddressINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type BlockHash = string
type FfiConverterTypeBlockHash = FfiConverterString
type FfiDestroyerTypeBlockHash = FfiDestroyerString

var FfiConverterTypeBlockHashINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type ChannelId = string
type FfiConverterTypeChannelId = FfiConverterString
type FfiDestroyerTypeChannelId = FfiDestroyerString

var FfiConverterTypeChannelIdINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type LSPS1OrderId = string
type FfiConverterTypeLSPS1OrderId = FfiConverterString
type FfiDestroyerTypeLSPS1OrderId = FfiDestroyerString

var FfiConverterTypeLSPS1OrderIdINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type LSPSDateTime = string
type FfiConverterTypeLSPSDateTime = FfiConverterString
type FfiDestroyerTypeLSPSDateTime = FfiDestroyerString

var FfiConverterTypeLSPSDateTimeINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type Mnemonic = string
type FfiConverterTypeMnemonic = FfiConverterString
type FfiDestroyerTypeMnemonic = FfiDestroyerString

var FfiConverterTypeMnemonicINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type NodeAlias = string
type FfiConverterTypeNodeAlias = FfiConverterString
type FfiDestroyerTypeNodeAlias = FfiDestroyerString

var FfiConverterTypeNodeAliasINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type NodeId = string
type FfiConverterTypeNodeId = FfiConverterString
type FfiDestroyerTypeNodeId = FfiDestroyerString

var FfiConverterTypeNodeIdINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type OfferId = string
type FfiConverterTypeOfferId = FfiConverterString
type FfiDestroyerTypeOfferId = FfiDestroyerString

var FfiConverterTypeOfferIdINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type PaymentHash = string
type FfiConverterTypePaymentHash = FfiConverterString
type FfiDestroyerTypePaymentHash = FfiDestroyerString

var FfiConverterTypePaymentHashINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type PaymentId = string
type FfiConverterTypePaymentId = FfiConverterString
type FfiDestroyerTypePaymentId = FfiDestroyerString

var FfiConverterTypePaymentIdINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type PaymentPreimage = string
type FfiConverterTypePaymentPreimage = FfiConverterString
type FfiDestroyerTypePaymentPreimage = FfiDestroyerString

var FfiConverterTypePaymentPreimageINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type PaymentSecret = string
type FfiConverterTypePaymentSecret = FfiConverterString
type FfiDestroyerTypePaymentSecret = FfiDestroyerString

var FfiConverterTypePaymentSecretINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type PublicKey = string
type FfiConverterTypePublicKey = FfiConverterString
type FfiDestroyerTypePublicKey = FfiDestroyerString

var FfiConverterTypePublicKeyINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type SocketAddress = string
type FfiConverterTypeSocketAddress = FfiConverterString
type FfiDestroyerTypeSocketAddress = FfiDestroyerString

var FfiConverterTypeSocketAddressINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type Txid = string
type FfiConverterTypeTxid = FfiConverterString
type FfiDestroyerTypeTxid = FfiDestroyerString

var FfiConverterTypeTxidINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type UntrustedString = string
type FfiConverterTypeUntrustedString = FfiConverterString
type FfiDestroyerTypeUntrustedString = FfiDestroyerString

var FfiConverterTypeUntrustedStringINSTANCE = FfiConverterString{}

/**
 * Typealias from the type name used in the UDL file to the builtin type.  This
 * is needed because the UDL type name is used in function/method signatures.
 * It's also what we have an external type that references a custom type.
 */
type UserChannelId = string
type FfiConverterTypeUserChannelId = FfiConverterString
type FfiDestroyerTypeUserChannelId = FfiDestroyerString

var FfiConverterTypeUserChannelIdINSTANCE = FfiConverterString{}

const (
	uniffiRustFuturePollReady      int8 = 0
	uniffiRustFuturePollMaybeReady int8 = 1
)

type rustFuturePollFunc func(C.uint64_t, C.UniffiRustFutureContinuationCallback, C.uint64_t)
type rustFutureCompleteFunc[T any] func(C.uint64_t, *C.RustCallStatus) T
type rustFutureFreeFunc func(C.uint64_t)

//export ldk_node_uniffiFutureContinuationCallback
func ldk_node_uniffiFutureContinuationCallback(data C.uint64_t, pollResult C.int8_t) {
	h := cgo.Handle(uintptr(data))
	waiter := h.Value().(chan int8)
	waiter <- int8(pollResult)
}

func uniffiRustCallAsync[E any, T any, F any](
	errConverter BufReader[*E],
	completeFunc rustFutureCompleteFunc[F],
	liftFunc func(F) T,
	rustFuture C.uint64_t,
	pollFunc rustFuturePollFunc,
	freeFunc rustFutureFreeFunc,
) (T, *E) {
	defer freeFunc(rustFuture)

	pollResult := int8(-1)
	waiter := make(chan int8, 1)

	chanHandle := cgo.NewHandle(waiter)
	defer chanHandle.Delete()

	for pollResult != uniffiRustFuturePollReady {
		pollFunc(
			rustFuture,
			(C.UniffiRustFutureContinuationCallback)(C.ldk_node_uniffiFutureContinuationCallback),
			C.uint64_t(chanHandle),
		)
		pollResult = <-waiter
	}

	var goValue T
	var ffiValue F
	var err *E

	ffiValue, err = rustCallWithError(errConverter, func(status *C.RustCallStatus) F {
		return completeFunc(rustFuture, status)
	})
	if err != nil {
		return goValue, err
	}
	return liftFunc(ffiValue), nil
}

//export ldk_node_uniffiFreeGorutine
func ldk_node_uniffiFreeGorutine(data C.uint64_t) {
	handle := cgo.Handle(uintptr(data))
	defer handle.Delete()

	guard := handle.Value().(chan struct{})
	guard <- struct{}{}
}

func DefaultConfig() Config {
	return FfiConverterConfigINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_func_default_config(_uniffiStatus),
		}
	}))
}

func GenerateEntropyMnemonic(wordCount *WordCount) Mnemonic {
	return FfiConverterTypeMnemonicINSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_func_generate_entropy_mnemonic(FfiConverterOptionalWordCountINSTANCE.Lower(wordCount), _uniffiStatus),
		}
	}))
}

func Lsps2ComputeOpeningFeeMsat(paymentSizeMsat uint64, openingFeeParams Lsps2OpeningFeeParams) *uint64 {
	return FfiConverterOptionalUint64INSTANCE.Lift(rustCall(func(_uniffiStatus *C.RustCallStatus) RustBufferI {
		return GoRustBuffer{
			inner: C.uniffi_ldk_node_fn_func_lsps2_compute_opening_fee_msat(FfiConverterUint64INSTANCE.Lower(paymentSizeMsat), FfiConverterLsps2OpeningFeeParamsINSTANCE.Lower(openingFeeParams), _uniffiStatus),
		}
	}))
}
