package iec61850

/*
#include <stdlib.h>
#include <goose_receiver.h>
#include <goose_subscriber.h>
#include <goose_publisher.h>
#include <linked_list.h>
#include <mms_value.h>

#ifndef GOOSE_FLAT_MMS_VALUE_DEFINED
#define GOOSE_FLAT_MMS_VALUE_DEFINED
typedef struct {
    int typ;
    int64_t ival;
    double dval;
    const char* sval;
} GooseFlatMmsValue;
#endif

extern void installGooseListener(GooseSubscriber subscriber, int handlerID);
extern GooseFlatMmsValue* flattenGooseDataSet(GooseSubscriber subscriber, int* outCount);
extern void destroyGooseDataSet(LinkedList dataSet);
extern CommParameters* createGooseCommParameters(uint8_t vlanPriority, uint16_t vlanId,
                                                 uint16_t appId, const uint8_t* destination);
*/
import "C"

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	gooseHandlerMu     sync.RWMutex
	gooseHandlers      = make(map[int]GooseHandler)
	nextGooseHandlerID atomic.Int32
)

type GooseReceiver struct {
	mu         sync.Mutex
	handle     C.GooseReceiver
	running    bool
	closed     bool
	handlerIDs []int
}

type GooseSubscriptionHandle struct {
	receiver   *GooseReceiver
	subscriber C.GooseSubscriber
	handlerID  int
	once       sync.Once
}

func NewGooseReceiver(interfaceName string) (*GooseReceiver, error) {
	if interfaceName == "" {
		return nil, errors.New("GOOSE receiver interface is required")
	}
	handle := C.GooseReceiver_create()
	if handle == nil {
		return nil, errors.New("create GOOSE receiver failed")
	}
	cInterface := C.CString(interfaceName)
	defer C.free(unsafe.Pointer(cInterface))
	C.GooseReceiver_setInterfaceId(handle, cInterface)
	return &GooseReceiver{handle: handle}, nil
}

func (r *GooseReceiver) AddSubscription(config GooseSubscription, handler GooseHandler) error {
	_, err := r.AddSubscriptionHandle(config, handler)
	return err
}

func (r *GooseReceiver) AddSubscriptionHandle(config GooseSubscription, handler GooseHandler) (*GooseSubscriptionHandle, error) {
	if config.GoCBRef == "" {
		return nil, errors.New("GOOSE control block reference is required")
	}
	if handler == nil {
		return nil, errors.New("GOOSE handler is required")
	}
	if len(config.Destination) != 0 && len(config.Destination) != 6 {
		return nil, fmt.Errorf("GOOSE destination MAC length=%d, want 6", len(config.Destination))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("GOOSE receiver is closed")
	}
	wasRunning := r.running
	if wasRunning {
		C.GooseReceiver_stop(r.handle)
		r.running = false
	}

	cRef := C.CString(config.GoCBRef)
	defer C.free(unsafe.Pointer(cRef))
	subscriber := C.GooseSubscriber_create(cRef, nil)
	if subscriber == nil {
		if wasRunning {
			C.GooseReceiver_start(r.handle)
			r.running = bool(C.GooseReceiver_isRunning(r.handle))
		}
		return nil, fmt.Errorf("create GOOSE subscriber %q failed", config.GoCBRef)
	}
	if config.APPID != 0 {
		C.GooseSubscriber_setAppId(subscriber, C.uint16_t(config.APPID))
	}
	if len(config.Destination) == 6 {
		C.GooseSubscriber_setDstMac(subscriber, (*C.uint8_t)(unsafe.Pointer(&config.Destination[0])))
	}

	handlerID := int(nextGooseHandlerID.Add(1))
	gooseHandlerMu.Lock()
	gooseHandlers[handlerID] = handler
	gooseHandlerMu.Unlock()
	C.installGooseListener(subscriber, C.int(handlerID))
	C.GooseReceiver_addSubscriber(r.handle, subscriber)
	r.handlerIDs = append(r.handlerIDs, handlerID)
	if wasRunning {
		C.GooseReceiver_start(r.handle)
		if !bool(C.GooseReceiver_isRunning(r.handle)) {
			C.GooseReceiver_removeSubscriber(r.handle, subscriber)
			C.GooseSubscriber_destroy(subscriber)
			r.handlerIDs = r.handlerIDs[:len(r.handlerIDs)-1]
			gooseHandlerMu.Lock()
			delete(gooseHandlers, handlerID)
			gooseHandlerMu.Unlock()
			C.GooseReceiver_start(r.handle)
			r.running = bool(C.GooseReceiver_isRunning(r.handle))
			return nil, errors.New("restart GOOSE receiver after adding subscription failed")
		}
		r.running = true
	}
	return &GooseSubscriptionHandle{receiver: r, subscriber: subscriber, handlerID: handlerID}, nil
}

func (h *GooseSubscriptionHandle) Close() {
	if h == nil || h.receiver == nil {
		return
	}
	h.once.Do(func() {
		r := h.receiver
		r.mu.Lock()
		if !r.closed {
			wasRunning := r.running
			if wasRunning {
				C.GooseReceiver_stop(r.handle)
				r.running = false
			}
			C.GooseReceiver_removeSubscriber(r.handle, h.subscriber)
			C.GooseSubscriber_destroy(h.subscriber)
			if wasRunning {
				C.GooseReceiver_start(r.handle)
				r.running = bool(C.GooseReceiver_isRunning(r.handle))
			}
		}
		r.mu.Unlock()
		gooseHandlerMu.Lock()
		delete(gooseHandlers, h.handlerID)
		gooseHandlerMu.Unlock()
	})
}

func (r *GooseReceiver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("GOOSE receiver is closed")
	}
	if r.running {
		return nil
	}
	C.GooseReceiver_start(r.handle)
	if !bool(C.GooseReceiver_isRunning(r.handle)) {
		return errors.New("start GOOSE receiver failed")
	}
	r.running = true
	return nil
}

func (r *GooseReceiver) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && r.running && bool(C.GooseReceiver_isRunning(r.handle))
}

func (r *GooseReceiver) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	if r.running {
		C.GooseReceiver_stop(r.handle)
		r.running = false
	}
	C.GooseReceiver_destroy(r.handle)
	r.handle = nil
	handlerIDs := append([]int(nil), r.handlerIDs...)
	r.handlerIDs = nil
	r.mu.Unlock()

	gooseHandlerMu.Lock()
	for _, handlerID := range handlerIDs {
		delete(gooseHandlers, handlerID)
	}
	gooseHandlerMu.Unlock()
}

//export goGooseCallback
func goGooseCallback(handlerID C.int, subscriberPtr unsafe.Pointer) {
	gooseHandlerMu.RLock()
	handler := gooseHandlers[int(handlerID)]
	gooseHandlerMu.RUnlock()
	if handler == nil {
		return
	}
	handler(buildGooseMessage(C.GooseSubscriber(subscriberPtr)))
}

func buildGooseMessage(subscriber C.GooseSubscriber) GooseMessage {
	message := GooseMessage{
		APPID:             uint16(C.GooseSubscriber_getAppId(subscriber)),
		StNum:             uint32(C.GooseSubscriber_getStNum(subscriber)),
		SqNum:             uint32(C.GooseSubscriber_getSqNum(subscriber)),
		ConfRev:           uint32(C.GooseSubscriber_getConfRev(subscriber)),
		TimeAllowedToLive: uint32(C.GooseSubscriber_getTimeAllowedToLive(subscriber)),
		Timestamp:         uint64(C.GooseSubscriber_getTimestamp(subscriber)),
		Valid:             bool(C.GooseSubscriber_isValid(subscriber)),
		Test:              bool(C.GooseSubscriber_isTest(subscriber)),
		NeedsCommission:   bool(C.GooseSubscriber_needsCommission(subscriber)),
		VLANSet:           bool(C.GooseSubscriber_isVlanSet(subscriber)),
		VLANID:            uint16(C.GooseSubscriber_getVlanId(subscriber)),
		VLANPriority:      uint8(C.GooseSubscriber_getVlanPrio(subscriber)),
		ParseError:        int(C.GooseSubscriber_getParseError(subscriber)),
	}
	if value := C.GooseSubscriber_getGoCbRef(subscriber); value != nil {
		message.GoCBRef = C.GoString(value)
	}
	if value := C.GooseSubscriber_getGoId(subscriber); value != nil {
		message.GoID = C.GoString(value)
	}
	if value := C.GooseSubscriber_getDataSet(subscriber); value != nil {
		message.DataSetRef = C.GoString(value)
	}
	message.SourceMAC = make(net.HardwareAddr, 6)
	message.DestinationMAC = make(net.HardwareAddr, 6)
	C.GooseSubscriber_getSrcMac(subscriber, (*C.uint8_t)(unsafe.Pointer(&message.SourceMAC[0])))
	C.GooseSubscriber_getDstMac(subscriber, (*C.uint8_t)(unsafe.Pointer(&message.DestinationMAC[0])))
	message.Values = flattenGooseValues(subscriber)
	return message
}

func flattenGooseValues(subscriber C.GooseSubscriber) []GoMmsValue {
	var count C.int
	values := C.flattenGooseDataSet(subscriber, &count)
	if values == nil || count <= 0 {
		return nil
	}
	defer C.free(unsafe.Pointer(values))
	flat := unsafe.Slice((*C.GooseFlatMmsValue)(unsafe.Pointer(values)), int(count))
	position := 0
	value := extractGooseFlatValue(flat, &position)
	if result, ok := value.Value.([]GoMmsValue); ok {
		return result
	}
	return []GoMmsValue{value}
}

func extractGooseFlatValue(values []C.GooseFlatMmsValue, position *int) GoMmsValue {
	if *position >= len(values) {
		return GoMmsValue{Type: MMSType(MMS_NIL)}
	}
	value := values[*position]
	*position++
	switch int(value.typ) {
	case int(MMS_BOOLEAN):
		return GoMmsValue{Type: MMS_BOOLEAN, Value: value.ival != 0}
	case int(MMS_FLOAT):
		return GoMmsValue{Type: MMS_FLOAT, Value: float64(value.dval)}
	case int(MMS_INTEGER):
		return GoMmsValue{Type: MMS_INTEGER, Value: int64(value.ival)}
	case int(MMS_UNSIGNED):
		return GoMmsValue{Type: MMS_UNSIGNED, Value: uint64(value.ival)}
	case int(MMS_STRING), int(MMS_VISIBLE_STRING), int(MMS_OCTET_STRING):
		if value.sval == nil {
			return GoMmsValue{Type: MMSType(value.typ), Value: ""}
		}
		return GoMmsValue{Type: MMSType(value.typ), Value: C.GoString(value.sval)}
	case int(MMS_BIT_STRING):
		return GoMmsValue{Type: MMS_BIT_STRING, Value: uint32(value.ival)}
	case int(MMS_UTC_TIME):
		return GoMmsValue{Type: MMS_UTC_TIME, Value: uint64(value.ival)}
	case 100, 102:
		typeValue := MMS_STRUCTURE
		end := 101
		if int(value.typ) == 102 {
			typeValue = MMS_ARRAY
			end = 103
		}
		children := make([]GoMmsValue, 0)
		for *position < len(values) && int(values[*position].typ) != end {
			children = append(children, extractGooseFlatValue(values, position))
		}
		if *position < len(values) {
			*position++
		}
		return GoMmsValue{Type: typeValue, Value: children}
	default:
		return GoMmsValue{Type: MMSType(value.typ)}
	}
}

type GoosePublisher struct {
	mu     sync.Mutex
	handle C.GoosePublisher
	closed bool
}

func NewGoosePublisher(config GoosePublisherConfig) (*GoosePublisher, error) {
	if config.InterfaceName == "" {
		return nil, errors.New("GOOSE publisher interface is required")
	}
	if len(config.Destination) != 6 {
		return nil, fmt.Errorf("GOOSE destination MAC length=%d, want 6", len(config.Destination))
	}
	if config.VLANID > 4095 || config.VLANPriority > 7 {
		return nil, fmt.Errorf("invalid GOOSE VLAN id=%d priority=%d", config.VLANID, config.VLANPriority)
	}
	parameters := C.createGooseCommParameters(
		C.uint8_t(config.VLANPriority), C.uint16_t(config.VLANID), C.uint16_t(config.APPID),
		(*C.uint8_t)(unsafe.Pointer(&config.Destination[0])),
	)
	if parameters == nil {
		return nil, errors.New("allocate GOOSE communication parameters failed")
	}
	defer C.free(unsafe.Pointer(parameters))
	cInterface := C.CString(config.InterfaceName)
	defer C.free(unsafe.Pointer(cInterface))
	handle := C.GoosePublisher_createEx(parameters, cInterface, C.bool(config.VLANSet))
	if handle == nil {
		return nil, errors.New("create GOOSE publisher failed")
	}
	publisher := &GoosePublisher{handle: handle}
	publisher.setStrings(config)
	C.GoosePublisher_setConfRev(handle, C.uint32_t(config.ConfRev))
	C.GoosePublisher_setTimeAllowedToLive(handle, C.uint32_t(config.TimeAllowedToLive))
	return publisher, nil
}

func (p *GoosePublisher) setStrings(config GoosePublisherConfig) {
	set := func(value string, apply func(*C.char)) {
		cValue := C.CString(value)
		defer C.free(unsafe.Pointer(cValue))
		apply(cValue)
	}
	set(config.GoCBRef, func(value *C.char) { C.GoosePublisher_setGoCbRef(p.handle, value) })
	set(config.GoID, func(value *C.char) { C.GoosePublisher_setGoID(p.handle, value) })
	set(config.DataSetRef, func(value *C.char) { C.GoosePublisher_setDataSetRef(p.handle, value) })
}

func (p *GoosePublisher) IncreaseState() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("GOOSE publisher is closed")
	}
	C.GoosePublisher_increaseStNum(p.handle)
	return nil
}

func (p *GoosePublisher) SetTimeAllowedToLive(ttl time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("GOOSE publisher is closed")
	}
	if ttl <= 0 || ttl > time.Duration(^uint32(0))*time.Millisecond {
		return fmt.Errorf("invalid GOOSE time allowed to live %v", ttl)
	}
	C.GoosePublisher_setTimeAllowedToLive(p.handle, C.uint32_t(ttl.Milliseconds()))
	return nil
}

func (p *GoosePublisher) Publish(values []GoMmsValue) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("GOOSE publisher is closed")
	}
	dataSet, err := newGooseDataSet(values)
	if err != nil {
		return err
	}
	defer C.destroyGooseDataSet(dataSet)
	if result := int(C.GoosePublisher_publish(p.handle, dataSet)); result != 0 {
		return fmt.Errorf("publish GOOSE failed: code=%d", result)
	}
	return nil
}

func (p *GoosePublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	C.GoosePublisher_destroy(p.handle)
	p.handle = nil
}

func newGooseDataSet(values []GoMmsValue) (C.LinkedList, error) {
	dataSet := C.LinkedList_create()
	if dataSet == nil {
		return nil, errors.New("create GOOSE data set failed")
	}
	for index, value := range values {
		mmsValue, err := newCMmsValue(value)
		if err != nil {
			C.destroyGooseDataSet(dataSet)
			return nil, fmt.Errorf("GOOSE data set member %d: %w", index, err)
		}
		C.LinkedList_add(dataSet, unsafe.Pointer(mmsValue))
	}
	return dataSet, nil
}

func newCMmsValue(value GoMmsValue) (*C.MmsValue, error) {
	switch value.Type {
	case MMS_BOOLEAN:
		parsed, ok := value.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("boolean value has type %T", value.Value)
		}
		return C.MmsValue_newBoolean(C.bool(parsed)), nil
	case MMS_INTEGER:
		parsed, ok := signedValue(value.Value)
		if !ok {
			return nil, fmt.Errorf("integer value has type %T", value.Value)
		}
		return C.MmsValue_newIntegerFromInt64(C.int64_t(parsed)), nil
	case MMS_UNSIGNED:
		parsed, ok := unsignedValue(value.Value)
		if !ok || parsed > uint64(^uint32(0)) {
			return nil, fmt.Errorf("unsigned value %v is outside uint32", value.Value)
		}
		return C.MmsValue_newUnsignedFromUint32(C.uint32_t(parsed)), nil
	case MMS_FLOAT:
		parsed, ok := floatValue(value.Value)
		if !ok {
			return nil, fmt.Errorf("float value has type %T", value.Value)
		}
		return C.MmsValue_newDouble(C.double(parsed)), nil
	case MMS_STRING, MMS_VISIBLE_STRING:
		parsed, ok := value.Value.(string)
		if !ok {
			return nil, fmt.Errorf("string value has type %T", value.Value)
		}
		cValue := C.CString(parsed)
		defer C.free(unsafe.Pointer(cValue))
		if value.Type == MMS_STRING {
			return C.MmsValue_newMmsString(cValue), nil
		}
		return C.MmsValue_newVisibleString(cValue), nil
	case MMS_BIT_STRING:
		parsed, ok := unsignedValue(value.Value)
		if !ok || parsed > uint64(^uint32(0)) {
			return nil, fmt.Errorf("bit string value %v is outside uint32", value.Value)
		}
		result := C.MmsValue_newBitString(32)
		C.MmsValue_setBitStringFromInteger(result, C.uint32_t(parsed))
		return result, nil
	case MMS_UTC_TIME:
		parsed, ok := unsignedValue(value.Value)
		if !ok {
			return nil, fmt.Errorf("UTC time value has type %T", value.Value)
		}
		return C.MmsValue_newUtcTimeByMsTime(C.uint64_t(parsed)), nil
	case MMS_ARRAY, MMS_STRUCTURE:
		children, ok := value.Value.([]GoMmsValue)
		if !ok {
			return nil, fmt.Errorf("complex value has type %T", value.Value)
		}
		var result *C.MmsValue
		if value.Type == MMS_ARRAY {
			result = C.MmsValue_createEmptyArray(C.int(len(children)))
		} else {
			result = C.MmsValue_createEmptyStructure(C.int(len(children)))
		}
		for index, child := range children {
			member, err := newCMmsValue(child)
			if err != nil {
				C.MmsValue_delete(result)
				return nil, fmt.Errorf("complex member %d: %w", index, err)
			}
			C.MmsValue_setElement(result, C.int(index), member)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported MMS type %d", value.Type)
	}
}

func signedValue(value interface{}) (int64, bool) {
	switch parsed := value.(type) {
	case int:
		return int64(parsed), true
	case int8:
		return int64(parsed), true
	case int16:
		return int64(parsed), true
	case int32:
		return int64(parsed), true
	case int64:
		return parsed, true
	}
	return 0, false
}

func unsignedValue(value interface{}) (uint64, bool) {
	switch parsed := value.(type) {
	case uint:
		return uint64(parsed), true
	case uint8:
		return uint64(parsed), true
	case uint16:
		return uint64(parsed), true
	case uint32:
		return uint64(parsed), true
	case uint64:
		return parsed, true
	}
	return 0, false
}

func floatValue(value interface{}) (float64, bool) {
	switch parsed := value.(type) {
	case float32:
		return float64(parsed), true
	case float64:
		return parsed, true
	}
	return 0, false
}
