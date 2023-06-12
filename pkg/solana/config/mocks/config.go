// Code generated by mockery v2.28.1. DO NOT EDIT.

package mocks

import (
	rpc "github.com/gagliardetto/solana-go/rpc"
	mock "github.com/stretchr/testify/mock"

	time "time"
)

// Config is an autogenerated mock type for the Config type
type Config struct {
	mock.Mock
}

// BalancePollPeriod provides a mock function with given fields:
func (_m *Config) BalancePollPeriod() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// BlockEmissionIdleWarningThreshold provides a mock function with given fields:
func (_m *Config) BlockEmissionIdleWarningThreshold() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// Commitment provides a mock function with given fields:
func (_m *Config) Commitment() rpc.CommitmentType {
	ret := _m.Called()

	var r0 rpc.CommitmentType
	if rf, ok := ret.Get(0).(func() rpc.CommitmentType); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(rpc.CommitmentType)
	}

	return r0
}

// ComputeUnitPriceDefault provides a mock function with given fields:
func (_m *Config) ComputeUnitPriceDefault() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// ComputeUnitPriceMax provides a mock function with given fields:
func (_m *Config) ComputeUnitPriceMax() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// ComputeUnitPriceMin provides a mock function with given fields:
func (_m *Config) ComputeUnitPriceMin() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// ConfirmPollPeriod provides a mock function with given fields:
func (_m *Config) ConfirmPollPeriod() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// FeeBumpPeriod provides a mock function with given fields:
func (_m *Config) FeeBumpPeriod() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// FeeEstimatorMode provides a mock function with given fields:
func (_m *Config) FeeEstimatorMode() string {
	ret := _m.Called()

	var r0 string
	if rf, ok := ret.Get(0).(func() string); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(string)
	}

	return r0
}

// FinalityDepth provides a mock function with given fields:
func (_m *Config) FinalityDepth() uint32 {
	ret := _m.Called()

	var r0 uint32
	if rf, ok := ret.Get(0).(func() uint32); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint32)
	}

	return r0
}

// HeadTrackerHistoryDepth provides a mock function with given fields:
func (_m *Config) HeadTrackerHistoryDepth() uint32 {
	ret := _m.Called()

	var r0 uint32
	if rf, ok := ret.Get(0).(func() uint32); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint32)
	}

	return r0
}

// HeadTrackerMaxBufferSize provides a mock function with given fields:
func (_m *Config) HeadTrackerMaxBufferSize() uint32 {
	ret := _m.Called()

	var r0 uint32
	if rf, ok := ret.Get(0).(func() uint32); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint32)
	}

	return r0
}

// HeadTrackerSamplingInterval provides a mock function with given fields:
func (_m *Config) HeadTrackerSamplingInterval() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// MaxRetries provides a mock function with given fields:
func (_m *Config) MaxRetries() *uint {
	ret := _m.Called()

	var r0 *uint
	if rf, ok := ret.Get(0).(func() *uint); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*uint)
		}
	}

	return r0
}

// OCR2CachePollPeriod provides a mock function with given fields:
func (_m *Config) OCR2CachePollPeriod() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// OCR2CacheTTL provides a mock function with given fields:
func (_m *Config) OCR2CacheTTL() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// SkipPreflight provides a mock function with given fields:
func (_m *Config) SkipPreflight() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// TxConfirmTimeout provides a mock function with given fields:
func (_m *Config) TxConfirmTimeout() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// TxRetryTimeout provides a mock function with given fields:
func (_m *Config) TxRetryTimeout() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

// TxTimeout provides a mock function with given fields:
func (_m *Config) TxTimeout() time.Duration {
	ret := _m.Called()

	var r0 time.Duration
	if rf, ok := ret.Get(0).(func() time.Duration); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(time.Duration)
	}

	return r0
}

type mockConstructorTestingTNewConfig interface {
	mock.TestingT
	Cleanup(func())
}

// NewConfig creates a new instance of Config. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewConfig(t mockConstructorTestingTNewConfig) *Config {
	mock := &Config{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
