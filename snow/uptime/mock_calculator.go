// Copyright (C) 2019-2022, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/lasthyphen/dijetsnodesgo/snow/uptime (interfaces: Calculator)

// Package uptime is a generated GoMock package.
package uptime

import (
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	ids "github.com/lasthyphen/dijetsnodesgo/ids"
)

// MockCalculator is a mock of Calculator interface.
type MockCalculator struct {
	ctrl     *gomock.Controller
	recorder *MockCalculatorMockRecorder
}

// MockCalculatorMockRecorder is the mock recorder for MockCalculator.
type MockCalculatorMockRecorder struct {
	mock *MockCalculator
}

// NewMockCalculator creates a new mock instance.
func NewMockCalculator(ctrl *gomock.Controller) *MockCalculator {
	mock := &MockCalculator{ctrl: ctrl}
	mock.recorder = &MockCalculatorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCalculator) EXPECT() *MockCalculatorMockRecorder {
	return m.recorder
}

// CalculateUptime mocks base method.
func (m *MockCalculator) CalculateUptime(arg0 ids.NodeID, arg1 ids.ID) (time.Duration, time.Time, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CalculateUptime", arg0, arg1)
	ret0, _ := ret[0].(time.Duration)
	ret1, _ := ret[1].(time.Time)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// CalculateUptime indicates an expected call of CalculateUptime.
func (mr *MockCalculatorMockRecorder) CalculateUptime(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CalculateUptime", reflect.TypeOf((*MockCalculator)(nil).CalculateUptime), arg0, arg1)
}

// CalculateUptimePercent mocks base method.
func (m *MockCalculator) CalculateUptimePercent(arg0 ids.NodeID, arg1 ids.ID) (float64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CalculateUptimePercent", arg0, arg1)
	ret0, _ := ret[0].(float64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CalculateUptimePercent indicates an expected call of CalculateUptimePercent.
func (mr *MockCalculatorMockRecorder) CalculateUptimePercent(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CalculateUptimePercent", reflect.TypeOf((*MockCalculator)(nil).CalculateUptimePercent), arg0, arg1)
}

// CalculateUptimePercentFrom mocks base method.
func (m *MockCalculator) CalculateUptimePercentFrom(arg0 ids.NodeID, arg1 ids.ID, arg2 time.Time) (float64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CalculateUptimePercentFrom", arg0, arg1, arg2)
	ret0, _ := ret[0].(float64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// CalculateUptimePercentFrom indicates an expected call of CalculateUptimePercentFrom.
func (mr *MockCalculatorMockRecorder) CalculateUptimePercentFrom(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CalculateUptimePercentFrom", reflect.TypeOf((*MockCalculator)(nil).CalculateUptimePercentFrom), arg0, arg1, arg2)
}
