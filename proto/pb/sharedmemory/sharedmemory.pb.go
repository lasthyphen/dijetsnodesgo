// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: sharedmemory/sharedmemory.proto

package sharedmemory

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BatchPut struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key   []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value []byte `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
}

func (x *BatchPut) Reset() {
	*x = BatchPut{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BatchPut) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BatchPut) ProtoMessage() {}

func (x *BatchPut) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BatchPut.ProtoReflect.Descriptor instead.
func (*BatchPut) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{0}
}

func (x *BatchPut) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *BatchPut) GetValue() []byte {
	if x != nil {
		return x.Value
	}
	return nil
}

type BatchDelete struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
}

func (x *BatchDelete) Reset() {
	*x = BatchDelete{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BatchDelete) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BatchDelete) ProtoMessage() {}

func (x *BatchDelete) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BatchDelete.ProtoReflect.Descriptor instead.
func (*BatchDelete) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{1}
}

func (x *BatchDelete) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

type Batch struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Puts    []*BatchPut    `protobuf:"bytes,1,rep,name=puts,proto3" json:"puts,omitempty"`
	Deletes []*BatchDelete `protobuf:"bytes,2,rep,name=deletes,proto3" json:"deletes,omitempty"`
	Id      int64          `protobuf:"varint,3,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *Batch) Reset() {
	*x = Batch{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Batch) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Batch) ProtoMessage() {}

func (x *Batch) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Batch.ProtoReflect.Descriptor instead.
func (*Batch) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{2}
}

func (x *Batch) GetPuts() []*BatchPut {
	if x != nil {
		return x.Puts
	}
	return nil
}

func (x *Batch) GetDeletes() []*BatchDelete {
	if x != nil {
		return x.Deletes
	}
	return nil
}

func (x *Batch) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

type AtomicRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RemoveRequests [][]byte   `protobuf:"bytes,1,rep,name=remove_requests,json=removeRequests,proto3" json:"remove_requests,omitempty"`
	PutRequests    []*Element `protobuf:"bytes,2,rep,name=put_requests,json=putRequests,proto3" json:"put_requests,omitempty"`
	PeerChainId    []byte     `protobuf:"bytes,3,opt,name=peer_chain_id,json=peerChainId,proto3" json:"peer_chain_id,omitempty"`
}

func (x *AtomicRequest) Reset() {
	*x = AtomicRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AtomicRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AtomicRequest) ProtoMessage() {}

func (x *AtomicRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AtomicRequest.ProtoReflect.Descriptor instead.
func (*AtomicRequest) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{3}
}

func (x *AtomicRequest) GetRemoveRequests() [][]byte {
	if x != nil {
		return x.RemoveRequests
	}
	return nil
}

func (x *AtomicRequest) GetPutRequests() []*Element {
	if x != nil {
		return x.PutRequests
	}
	return nil
}

func (x *AtomicRequest) GetPeerChainId() []byte {
	if x != nil {
		return x.PeerChainId
	}
	return nil
}

type Element struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Key    []byte   `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value  []byte   `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	Traits [][]byte `protobuf:"bytes,3,rep,name=traits,proto3" json:"traits,omitempty"`
}

func (x *Element) Reset() {
	*x = Element{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Element) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Element) ProtoMessage() {}

func (x *Element) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Element.ProtoReflect.Descriptor instead.
func (*Element) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{4}
}

func (x *Element) GetKey() []byte {
	if x != nil {
		return x.Key
	}
	return nil
}

func (x *Element) GetValue() []byte {
	if x != nil {
		return x.Value
	}
	return nil
}

func (x *Element) GetTraits() [][]byte {
	if x != nil {
		return x.Traits
	}
	return nil
}

type GetRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PeerChainId []byte   `protobuf:"bytes,1,opt,name=peer_chain_id,json=peerChainId,proto3" json:"peer_chain_id,omitempty"`
	Keys        [][]byte `protobuf:"bytes,2,rep,name=keys,proto3" json:"keys,omitempty"`
	Id          int64    `protobuf:"varint,3,opt,name=id,proto3" json:"id,omitempty"`
	Continues   bool     `protobuf:"varint,4,opt,name=continues,proto3" json:"continues,omitempty"`
}

func (x *GetRequest) Reset() {
	*x = GetRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetRequest) ProtoMessage() {}

func (x *GetRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetRequest.ProtoReflect.Descriptor instead.
func (*GetRequest) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{5}
}

func (x *GetRequest) GetPeerChainId() []byte {
	if x != nil {
		return x.PeerChainId
	}
	return nil
}

func (x *GetRequest) GetKeys() [][]byte {
	if x != nil {
		return x.Keys
	}
	return nil
}

func (x *GetRequest) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *GetRequest) GetContinues() bool {
	if x != nil {
		return x.Continues
	}
	return false
}

type GetResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Values    [][]byte `protobuf:"bytes,1,rep,name=values,proto3" json:"values,omitempty"`
	Continues bool     `protobuf:"varint,2,opt,name=continues,proto3" json:"continues,omitempty"`
}

func (x *GetResponse) Reset() {
	*x = GetResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetResponse) ProtoMessage() {}

func (x *GetResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetResponse.ProtoReflect.Descriptor instead.
func (*GetResponse) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{6}
}

func (x *GetResponse) GetValues() [][]byte {
	if x != nil {
		return x.Values
	}
	return nil
}

func (x *GetResponse) GetContinues() bool {
	if x != nil {
		return x.Continues
	}
	return false
}

type IndexedRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	PeerChainId []byte   `protobuf:"bytes,1,opt,name=peer_chain_id,json=peerChainId,proto3" json:"peer_chain_id,omitempty"`
	Traits      [][]byte `protobuf:"bytes,2,rep,name=traits,proto3" json:"traits,omitempty"`
	StartTrait  []byte   `protobuf:"bytes,3,opt,name=start_trait,json=startTrait,proto3" json:"start_trait,omitempty"`
	StartKey    []byte   `protobuf:"bytes,4,opt,name=start_key,json=startKey,proto3" json:"start_key,omitempty"`
	Limit       int32    `protobuf:"varint,5,opt,name=limit,proto3" json:"limit,omitempty"`
	Id          int64    `protobuf:"varint,6,opt,name=id,proto3" json:"id,omitempty"`
	Continues   bool     `protobuf:"varint,7,opt,name=continues,proto3" json:"continues,omitempty"`
}

func (x *IndexedRequest) Reset() {
	*x = IndexedRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IndexedRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IndexedRequest) ProtoMessage() {}

func (x *IndexedRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IndexedRequest.ProtoReflect.Descriptor instead.
func (*IndexedRequest) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{7}
}

func (x *IndexedRequest) GetPeerChainId() []byte {
	if x != nil {
		return x.PeerChainId
	}
	return nil
}

func (x *IndexedRequest) GetTraits() [][]byte {
	if x != nil {
		return x.Traits
	}
	return nil
}

func (x *IndexedRequest) GetStartTrait() []byte {
	if x != nil {
		return x.StartTrait
	}
	return nil
}

func (x *IndexedRequest) GetStartKey() []byte {
	if x != nil {
		return x.StartKey
	}
	return nil
}

func (x *IndexedRequest) GetLimit() int32 {
	if x != nil {
		return x.Limit
	}
	return 0
}

func (x *IndexedRequest) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *IndexedRequest) GetContinues() bool {
	if x != nil {
		return x.Continues
	}
	return false
}

type IndexedResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Values    [][]byte `protobuf:"bytes,1,rep,name=values,proto3" json:"values,omitempty"`
	LastTrait []byte   `protobuf:"bytes,2,opt,name=last_trait,json=lastTrait,proto3" json:"last_trait,omitempty"`
	LastKey   []byte   `protobuf:"bytes,3,opt,name=last_key,json=lastKey,proto3" json:"last_key,omitempty"`
	Continues bool     `protobuf:"varint,4,opt,name=continues,proto3" json:"continues,omitempty"`
}

func (x *IndexedResponse) Reset() {
	*x = IndexedResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IndexedResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IndexedResponse) ProtoMessage() {}

func (x *IndexedResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IndexedResponse.ProtoReflect.Descriptor instead.
func (*IndexedResponse) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{8}
}

func (x *IndexedResponse) GetValues() [][]byte {
	if x != nil {
		return x.Values
	}
	return nil
}

func (x *IndexedResponse) GetLastTrait() []byte {
	if x != nil {
		return x.LastTrait
	}
	return nil
}

func (x *IndexedResponse) GetLastKey() []byte {
	if x != nil {
		return x.LastKey
	}
	return nil
}

func (x *IndexedResponse) GetContinues() bool {
	if x != nil {
		return x.Continues
	}
	return false
}

type ApplyRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Requests  []*AtomicRequest `protobuf:"bytes,1,rep,name=requests,proto3" json:"requests,omitempty"`
	Batches   []*Batch         `protobuf:"bytes,2,rep,name=batches,proto3" json:"batches,omitempty"`
	Id        int64            `protobuf:"varint,3,opt,name=id,proto3" json:"id,omitempty"`
	Continues bool             `protobuf:"varint,4,opt,name=continues,proto3" json:"continues,omitempty"`
}

func (x *ApplyRequest) Reset() {
	*x = ApplyRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ApplyRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ApplyRequest) ProtoMessage() {}

func (x *ApplyRequest) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ApplyRequest.ProtoReflect.Descriptor instead.
func (*ApplyRequest) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{9}
}

func (x *ApplyRequest) GetRequests() []*AtomicRequest {
	if x != nil {
		return x.Requests
	}
	return nil
}

func (x *ApplyRequest) GetBatches() []*Batch {
	if x != nil {
		return x.Batches
	}
	return nil
}

func (x *ApplyRequest) GetId() int64 {
	if x != nil {
		return x.Id
	}
	return 0
}

func (x *ApplyRequest) GetContinues() bool {
	if x != nil {
		return x.Continues
	}
	return false
}

type ApplyResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ApplyResponse) Reset() {
	*x = ApplyResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_sharedmemory_sharedmemory_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ApplyResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ApplyResponse) ProtoMessage() {}

func (x *ApplyResponse) ProtoReflect() protoreflect.Message {
	mi := &file_sharedmemory_sharedmemory_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ApplyResponse.ProtoReflect.Descriptor instead.
func (*ApplyResponse) Descriptor() ([]byte, []int) {
	return file_sharedmemory_sharedmemory_proto_rawDescGZIP(), []int{10}
}

var File_sharedmemory_sharedmemory_proto protoreflect.FileDescriptor

var file_sharedmemory_sharedmemory_proto_rawDesc = []byte{
	0x0a, 0x1f, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2f, 0x73,
	0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x0c, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x22,
	0x32, 0x0a, 0x08, 0x42, 0x61, 0x74, 0x63, 0x68, 0x50, 0x75, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6b,
	0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a,
	0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x05, 0x76, 0x61,
	0x6c, 0x75, 0x65, 0x22, 0x1f, 0x0a, 0x0b, 0x42, 0x61, 0x74, 0x63, 0x68, 0x44, 0x65, 0x6c, 0x65,
	0x74, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x03, 0x6b, 0x65, 0x79, 0x22, 0x78, 0x0a, 0x05, 0x42, 0x61, 0x74, 0x63, 0x68, 0x12, 0x2a, 0x0a,
	0x04, 0x70, 0x75, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x16, 0x2e, 0x73, 0x68,
	0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x42, 0x61, 0x74, 0x63, 0x68,
	0x50, 0x75, 0x74, 0x52, 0x04, 0x70, 0x75, 0x74, 0x73, 0x12, 0x33, 0x0a, 0x07, 0x64, 0x65, 0x6c,
	0x65, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x73, 0x68, 0x61,
	0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x42, 0x61, 0x74, 0x63, 0x68, 0x44,
	0x65, 0x6c, 0x65, 0x74, 0x65, 0x52, 0x07, 0x64, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x73, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x22, 0x96,
	0x01, 0x0a, 0x0d, 0x41, 0x74, 0x6f, 0x6d, 0x69, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x27, 0x0a, 0x0f, 0x72, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x5f, 0x72, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x0e, 0x72, 0x65, 0x6d, 0x6f, 0x76,
	0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x73, 0x12, 0x38, 0x0a, 0x0c, 0x70, 0x75, 0x74,
	0x5f, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x15, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x45,
	0x6c, 0x65, 0x6d, 0x65, 0x6e, 0x74, 0x52, 0x0b, 0x70, 0x75, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x73, 0x12, 0x22, 0x0a, 0x0d, 0x70, 0x65, 0x65, 0x72, 0x5f, 0x63, 0x68, 0x61, 0x69,
	0x6e, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b, 0x70, 0x65, 0x65, 0x72,
	0x43, 0x68, 0x61, 0x69, 0x6e, 0x49, 0x64, 0x22, 0x49, 0x0a, 0x07, 0x45, 0x6c, 0x65, 0x6d, 0x65,
	0x6e, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x74, 0x72,
	0x61, 0x69, 0x74, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x06, 0x74, 0x72, 0x61, 0x69,
	0x74, 0x73, 0x22, 0x72, 0x0a, 0x0a, 0x47, 0x65, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x12, 0x22, 0x0a, 0x0d, 0x70, 0x65, 0x65, 0x72, 0x5f, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x5f, 0x69,
	0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b, 0x70, 0x65, 0x65, 0x72, 0x43, 0x68, 0x61,
	0x69, 0x6e, 0x49, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x18, 0x02, 0x20, 0x03,
	0x28, 0x0c, 0x52, 0x04, 0x6b, 0x65, 0x79, 0x73, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6e, 0x74,
	0x69, 0x6e, 0x75, 0x65, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x63, 0x6f, 0x6e,
	0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x22, 0x43, 0x0a, 0x0b, 0x47, 0x65, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x18,
	0x01, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x12, 0x1c, 0x0a,
	0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x22, 0xce, 0x01, 0x0a, 0x0e,
	0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x22,
	0x0a, 0x0d, 0x70, 0x65, 0x65, 0x72, 0x5f, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x5f, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0b, 0x70, 0x65, 0x65, 0x72, 0x43, 0x68, 0x61, 0x69, 0x6e,
	0x49, 0x64, 0x12, 0x16, 0x0a, 0x06, 0x74, 0x72, 0x61, 0x69, 0x74, 0x73, 0x18, 0x02, 0x20, 0x03,
	0x28, 0x0c, 0x52, 0x06, 0x74, 0x72, 0x61, 0x69, 0x74, 0x73, 0x12, 0x1f, 0x0a, 0x0b, 0x73, 0x74,
	0x61, 0x72, 0x74, 0x5f, 0x74, 0x72, 0x61, 0x69, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x0a, 0x73, 0x74, 0x61, 0x72, 0x74, 0x54, 0x72, 0x61, 0x69, 0x74, 0x12, 0x1b, 0x0a, 0x09, 0x73,
	0x74, 0x61, 0x72, 0x74, 0x5f, 0x6b, 0x65, 0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x08,
	0x73, 0x74, 0x61, 0x72, 0x74, 0x4b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x6c, 0x69, 0x6d, 0x69,
	0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x05, 0x52, 0x05, 0x6c, 0x69, 0x6d, 0x69, 0x74, 0x12, 0x0e,
	0x0a, 0x02, 0x69, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1c,
	0x0a, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x18, 0x07, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x22, 0x81, 0x01, 0x0a,
	0x0f, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x12, 0x16, 0x0a, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0c,
	0x52, 0x06, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x73, 0x12, 0x1d, 0x0a, 0x0a, 0x6c, 0x61, 0x73, 0x74,
	0x5f, 0x74, 0x72, 0x61, 0x69, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x6c, 0x61,
	0x73, 0x74, 0x54, 0x72, 0x61, 0x69, 0x74, 0x12, 0x19, 0x0a, 0x08, 0x6c, 0x61, 0x73, 0x74, 0x5f,
	0x6b, 0x65, 0x79, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x6c, 0x61, 0x73, 0x74, 0x4b,
	0x65, 0x79, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x63, 0x6f, 0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73,
	0x22, 0xa4, 0x01, 0x0a, 0x0c, 0x41, 0x70, 0x70, 0x6c, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x12, 0x37, 0x0a, 0x08, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x0b, 0x32, 0x1b, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f,
	0x72, 0x79, 0x2e, 0x41, 0x74, 0x6f, 0x6d, 0x69, 0x63, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x52, 0x08, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x73, 0x12, 0x2d, 0x0a, 0x07, 0x62, 0x61,
	0x74, 0x63, 0x68, 0x65, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x13, 0x2e, 0x73, 0x68,
	0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x42, 0x61, 0x74, 0x63, 0x68,
	0x52, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x65, 0x73, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x02, 0x69, 0x64, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6e,
	0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x63, 0x6f,
	0x6e, 0x74, 0x69, 0x6e, 0x75, 0x65, 0x73, 0x22, 0x0f, 0x0a, 0x0d, 0x41, 0x70, 0x70, 0x6c, 0x79,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0xd4, 0x01, 0x0a, 0x0c, 0x53, 0x68, 0x61,
	0x72, 0x65, 0x64, 0x4d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x12, 0x3a, 0x0a, 0x03, 0x47, 0x65, 0x74,
	0x12, 0x18, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e,
	0x47, 0x65, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x19, 0x2e, 0x73, 0x68, 0x61,
	0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x47, 0x65, 0x74, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x46, 0x0a, 0x07, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x64,
	0x12, 0x1c, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e,
	0x49, 0x6e, 0x64, 0x65, 0x78, 0x65, 0x64, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1d,
	0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x49, 0x6e,
	0x64, 0x65, 0x78, 0x65, 0x64, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x40, 0x0a,
	0x05, 0x41, 0x70, 0x70, 0x6c, 0x79, 0x12, 0x1a, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d,
	0x65, 0x6d, 0x6f, 0x72, 0x79, 0x2e, 0x41, 0x70, 0x70, 0x6c, 0x79, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x1b, 0x2e, 0x73, 0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72,
	0x79, 0x2e, 0x41, 0x70, 0x70, 0x6c, 0x79, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x42,
	0x3a, 0x5a, 0x38, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6c, 0x61,
	0x73, 0x74, 0x68, 0x79, 0x70, 0x68, 0x65, 0x6e, 0x2f, 0x64, 0x69, 0x6a, 0x65, 0x74, 0x73, 0x6e,
	0x6f, 0x64, 0x65, 0x67, 0x6f, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x70, 0x62, 0x2f, 0x73,
	0x68, 0x61, 0x72, 0x65, 0x64, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_sharedmemory_sharedmemory_proto_rawDescOnce sync.Once
	file_sharedmemory_sharedmemory_proto_rawDescData = file_sharedmemory_sharedmemory_proto_rawDesc
)

func file_sharedmemory_sharedmemory_proto_rawDescGZIP() []byte {
	file_sharedmemory_sharedmemory_proto_rawDescOnce.Do(func() {
		file_sharedmemory_sharedmemory_proto_rawDescData = protoimpl.X.CompressGZIP(file_sharedmemory_sharedmemory_proto_rawDescData)
	})
	return file_sharedmemory_sharedmemory_proto_rawDescData
}

var file_sharedmemory_sharedmemory_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_sharedmemory_sharedmemory_proto_goTypes = []interface{}{
	(*BatchPut)(nil),        // 0: sharedmemory.BatchPut
	(*BatchDelete)(nil),     // 1: sharedmemory.BatchDelete
	(*Batch)(nil),           // 2: sharedmemory.Batch
	(*AtomicRequest)(nil),   // 3: sharedmemory.AtomicRequest
	(*Element)(nil),         // 4: sharedmemory.Element
	(*GetRequest)(nil),      // 5: sharedmemory.GetRequest
	(*GetResponse)(nil),     // 6: sharedmemory.GetResponse
	(*IndexedRequest)(nil),  // 7: sharedmemory.IndexedRequest
	(*IndexedResponse)(nil), // 8: sharedmemory.IndexedResponse
	(*ApplyRequest)(nil),    // 9: sharedmemory.ApplyRequest
	(*ApplyResponse)(nil),   // 10: sharedmemory.ApplyResponse
}
var file_sharedmemory_sharedmemory_proto_depIdxs = []int32{
	0,  // 0: sharedmemory.Batch.puts:type_name -> sharedmemory.BatchPut
	1,  // 1: sharedmemory.Batch.deletes:type_name -> sharedmemory.BatchDelete
	4,  // 2: sharedmemory.AtomicRequest.put_requests:type_name -> sharedmemory.Element
	3,  // 3: sharedmemory.ApplyRequest.requests:type_name -> sharedmemory.AtomicRequest
	2,  // 4: sharedmemory.ApplyRequest.batches:type_name -> sharedmemory.Batch
	5,  // 5: sharedmemory.SharedMemory.Get:input_type -> sharedmemory.GetRequest
	7,  // 6: sharedmemory.SharedMemory.Indexed:input_type -> sharedmemory.IndexedRequest
	9,  // 7: sharedmemory.SharedMemory.Apply:input_type -> sharedmemory.ApplyRequest
	6,  // 8: sharedmemory.SharedMemory.Get:output_type -> sharedmemory.GetResponse
	8,  // 9: sharedmemory.SharedMemory.Indexed:output_type -> sharedmemory.IndexedResponse
	10, // 10: sharedmemory.SharedMemory.Apply:output_type -> sharedmemory.ApplyResponse
	8,  // [8:11] is the sub-list for method output_type
	5,  // [5:8] is the sub-list for method input_type
	5,  // [5:5] is the sub-list for extension type_name
	5,  // [5:5] is the sub-list for extension extendee
	0,  // [0:5] is the sub-list for field type_name
}

func init() { file_sharedmemory_sharedmemory_proto_init() }
func file_sharedmemory_sharedmemory_proto_init() {
	if File_sharedmemory_sharedmemory_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_sharedmemory_sharedmemory_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BatchPut); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BatchDelete); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Batch); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AtomicRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Element); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*IndexedRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*IndexedResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ApplyRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_sharedmemory_sharedmemory_proto_msgTypes[10].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ApplyResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_sharedmemory_sharedmemory_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_sharedmemory_sharedmemory_proto_goTypes,
		DependencyIndexes: file_sharedmemory_sharedmemory_proto_depIdxs,
		MessageInfos:      file_sharedmemory_sharedmemory_proto_msgTypes,
	}.Build()
	File_sharedmemory_sharedmemory_proto = out.File
	file_sharedmemory_sharedmemory_proto_rawDesc = nil
	file_sharedmemory_sharedmemory_proto_goTypes = nil
	file_sharedmemory_sharedmemory_proto_depIdxs = nil
}
