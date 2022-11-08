// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/ory/hydra/x (interfaces: CookieConfigProvider)

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	http "net/http"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockCookieConfigProvider is a mock of CookieConfigProvider interface.
type MockCookieConfigProvider struct {
	ctrl     *gomock.Controller
	recorder *MockCookieConfigProviderMockRecorder
}

// MockCookieConfigProviderMockRecorder is the mock recorder for MockCookieConfigProvider.
type MockCookieConfigProviderMockRecorder struct {
	mock *MockCookieConfigProvider
}

// NewMockCookieConfigProvider creates a new mock instance.
func NewMockCookieConfigProvider(ctrl *gomock.Controller) *MockCookieConfigProvider {
	mock := &MockCookieConfigProvider{ctrl: ctrl}
	mock.recorder = &MockCookieConfigProviderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCookieConfigProvider) EXPECT() *MockCookieConfigProviderMockRecorder {
	return m.recorder
}

// CookieDomain mocks base method.
func (m *MockCookieConfigProvider) CookieDomain(arg0 context.Context) string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CookieDomain", arg0)
	ret0, _ := ret[0].(string)
	return ret0
}

// CookieDomain indicates an expected call of CookieDomain.
func (mr *MockCookieConfigProviderMockRecorder) CookieDomain(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CookieDomain", reflect.TypeOf((*MockCookieConfigProvider)(nil).CookieDomain), arg0)
}

// CookieSameSiteLegacyWorkaround mocks base method.
func (m *MockCookieConfigProvider) CookieSameSiteLegacyWorkaround(arg0 context.Context) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CookieSameSiteLegacyWorkaround", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// CookieSameSiteLegacyWorkaround indicates an expected call of CookieSameSiteLegacyWorkaround.
func (mr *MockCookieConfigProviderMockRecorder) CookieSameSiteLegacyWorkaround(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CookieSameSiteLegacyWorkaround", reflect.TypeOf((*MockCookieConfigProvider)(nil).CookieSameSiteLegacyWorkaround), arg0)
}

// CookieSameSiteMode mocks base method.
func (m *MockCookieConfigProvider) CookieSameSiteMode(arg0 context.Context) http.SameSite {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CookieSameSiteMode", arg0)
	ret0, _ := ret[0].(http.SameSite)
	return ret0
}

// CookieSameSiteMode indicates an expected call of CookieSameSiteMode.
func (mr *MockCookieConfigProviderMockRecorder) CookieSameSiteMode(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CookieSameSiteMode", reflect.TypeOf((*MockCookieConfigProvider)(nil).CookieSameSiteMode), arg0)
}

// CookieSecure mocks base method.
func (m *MockCookieConfigProvider) CookieSecure(arg0 context.Context) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "CookieSecure", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// CookieSecure indicates an expected call of CookieSecure.
func (mr *MockCookieConfigProviderMockRecorder) CookieSecure(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "CookieSecure", reflect.TypeOf((*MockCookieConfigProvider)(nil).CookieSecure), arg0)
}

// IsDevelopmentMode mocks base method.
func (m *MockCookieConfigProvider) IsDevelopmentMode(arg0 context.Context) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IsDevelopmentMode", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// IsDevelopmentMode indicates an expected call of IsDevelopmentMode.
func (mr *MockCookieConfigProviderMockRecorder) IsDevelopmentMode(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IsDevelopmentMode", reflect.TypeOf((*MockCookieConfigProvider)(nil).IsDevelopmentMode), arg0)
}