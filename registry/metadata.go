//
//
// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2023 Tencent.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.
//
//

package registry

import (
	"fmt"
	"reflect"
	"sync"
	"unicode/utf8"
)

const (
	maxRegisterMetadataEntries = 64
	maxRegisterMetadataValue   = 65535
)

var reservedAgentMetadataKeys = []string{
	"ai-agent-name",
	"a2a.agent.card",
	"a2a.transport",
	"a2a.path",
}

var metadataProviders sync.Map

// MetadataProvider supplies dynamic instance metadata when a service is registered.
type MetadataProvider interface {
	Metadata() (map[string]string, error)
}

// MetadataProviderFunc adapts a function to MetadataProvider.
type MetadataProviderFunc func() (map[string]string, error)

// Metadata calls f.
func (f MetadataProviderFunc) Metadata() (map[string]string, error) {
	if f == nil {
		return nil, fmt.Errorf("metadata provider func must not be nil")
	}
	return f()
}

// RegisterMetadataProvider registers a dynamic metadata provider for one service.
func RegisterMetadataProvider(service string, provider MetadataProvider) error {
	if service == "" {
		return fmt.Errorf("metadata provider service must not be empty")
	}
	if isNilMetadataProvider(provider) {
		return fmt.Errorf("metadata provider for service %q must not be nil", service)
	}
	if _, loaded := metadataProviders.LoadOrStore(service, provider); loaded {
		return fmt.Errorf("metadata provider for service %q is already registered", service)
	}
	return nil
}

// UnregisterMetadataProvider removes the dynamic metadata provider for one service.
func UnregisterMetadataProvider(service string) {
	metadataProviders.Delete(service)
}

func getMetadataProvider(service string) (MetadataProvider, bool) {
	provider, ok := metadataProviders.Load(service)
	if !ok {
		return nil, false
	}
	return provider.(MetadataProvider), true
}

func isNilMetadataProvider(provider MetadataProvider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func validateRegisterMetadata(metadata map[string]string) error {
	if len(metadata) > maxRegisterMetadataEntries {
		return fmt.Errorf("register metadata contains %d entries, maximum is %d",
			len(metadata), maxRegisterMetadataEntries)
	}
	for key, value := range metadata {
		if key == "" {
			return fmt.Errorf("register metadata key must not be empty")
		}
		if length := utf8.RuneCountInString(value); length > maxRegisterMetadataValue {
			return fmt.Errorf("register metadata value for key %q contains %d characters, maximum is %d",
				key, length, maxRegisterMetadataValue)
		}
	}
	return nil
}
