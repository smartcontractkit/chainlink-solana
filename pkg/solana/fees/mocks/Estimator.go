// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
)

// Estimator is an autogenerated mock type for the Estimator type
type Estimator struct {
	mock.Mock
}

// BaseComputeUnitPrice provides a mock function with given fields:
func (_m *Estimator) BaseComputeUnitPrice() uint64 {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for BaseComputeUnitPrice")
	}

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// Close provides a mock function with given fields:
func (_m *Estimator) Close() error {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for Close")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Start provides a mock function with given fields: _a0
func (_m *Estimator) Start(_a0 context.Context) error {
	ret := _m.Called(_a0)

	if len(ret) == 0 {
		panic("no return value specified for Start")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewEstimator creates a new instance of Estimator. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewEstimator(t interface {
	mock.TestingT
	Cleanup(func())
}) *Estimator {
	mock := &Estimator{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
