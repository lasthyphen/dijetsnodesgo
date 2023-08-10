// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lasthyphen/dijetsnodesgo/vms/platformvm/state (interfaces: Versions)

// Package state is a generated GoMock package.
package state

import (
	reflect "reflect"

	ids "github.com/lasthyphen/dijetsnodesgo/ids"
	gomock "github.com/golang/mock/gomock"
)

// MockVersions is a mock of Versions interface.
type MockVersions struct {
	ctrl     *gomock.Controller
	recorder *MockVersionsMockRecorder
}

// MockVersionsMockRecorder is the mock recorder for MockVersions.
type MockVersionsMockRecorder struct {
	mock *MockVersions
}

// NewMockVersions creates a new mock instance.
func NewMockVersions(ctrl *gomock.Controller) *MockVersions {
	mock := &MockVersions{ctrl: ctrl}
	mock.recorder = &MockVersionsMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockVersions) EXPECT() *MockVersionsMockRecorder {
	return m.recorder
}

// GetState mocks base method.
func (m *MockVersions) GetState(arg0 ids.ID) (Chain, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetState", arg0)
	ret0, _ := ret[0].(Chain)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// GetState indicates an expected call of GetState.
func (mr *MockVersionsMockRecorder) GetState(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetState", reflect.TypeOf((*MockVersions)(nil).GetState), arg0)
}
