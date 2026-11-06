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

// Package registry is for service registry.
package registry

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"trpc.group/trpc-go/trpc-go/healthcheck"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/metrics"

	"github.com/polarismesh/polaris-go/api"
	plog "github.com/polarismesh/polaris-go/pkg/log"
	"github.com/polarismesh/polaris-go/pkg/model"
)

const (
	defaultHeartBeat = 5
	defaultTTL       = 5
)

var defaultWeight = 100

// Registry is service registration.
type Registry struct {
	// Provider: public for user custom.
	Provider api.ProviderAPI
	cfg      *Config
	sdkCtx   api.SDKContext
	host     string
	port     int
	statuses chan healthcheck.Status
}

// newRegistry is to new an instance.
func newRegistry(provider api.ProviderAPI, cfg *Config) (*Registry, error) {
	if cfg.HeartBeat == 0 {
		cfg.HeartBeat = defaultHeartBeat
	}
	if cfg.Weight == nil {
		cfg.Weight = &defaultWeight
	}
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
	statuses := make(chan healthcheck.Status, 1)
	healthcheck.Watch(cfg.ServiceName, func(status healthcheck.Status) {
		for {
			select {
			case statuses <- status:
				return
			default:
				// this function should not block healthcheck goroutine.
				// when statuses is full, consume the oldest one to make room for the new status.
				<-statuses
			}
		}
	})
	return &Registry{
		Provider: provider,
		cfg:      cfg,
		statuses: statuses,
	}, nil
}

// NewRegistry provides an externally accessible new instance interface.
func NewRegistry(provider api.ProviderAPI, cfg *Config) (*Registry, error) {
	return newRegistry(provider, cfg)
}

// Register is for registration service.
func (r *Registry) Register(_ string, opt ...registry.Option) error {
	opts := &registry.Options{}
	for _, o := range opt {
		o(opts)
	}

	address := opts.Address
	if address == "" || r.cfg.PreferBindAddress {
		address = r.cfg.BindAddress
		address = os.ExpandEnv(address) // also allow environ
	}

	host, portRaw, _ := net.SplitHostPort(address)
	port, _ := strconv.ParseInt(portRaw, 10, 64)
	r.host = host
	r.port = int(port)
	if r.cfg.EnableRegister {
		if err := r.register(); err != nil {
			return err
		}
	}
	go r.heartBeats()
	return nil
}

func (r *Registry) register() error {
	metadata, err := r.buildRegisterMetadata()
	if err != nil {
		return err
	}
	req := &api.InstanceRegisterRequest{
		InstanceRegisterRequest: model.InstanceRegisterRequest{
			Namespace:    r.cfg.Namespace,
			Service:      r.cfg.ServiceName,
			Host:         r.host,
			Port:         r.port,
			ServiceToken: r.cfg.ServiceToken,
			Weight:       r.cfg.Weight,
			Metadata:     metadata,
			Location:     r.cfg.InstanceLocation,
		},
	}
	if r.cfg.Version != "" {
		version := r.cfg.Version
		req.Version = &version
	}
	if !r.cfg.DisableHealthCheck {
		req.SetTTL(r.cfg.TTL)
	}
	resp, err := r.Provider.Register(req)
	if err != nil {
		return fmt.Errorf("fail to Register instance, err is %v", err)
	}
	plog.GetBaseLogger().Debugf("success to register instance1, id is %s\n", resp.InstanceID)
	r.cfg.InstanceID = resp.InstanceID
	return nil
}

func (r *Registry) buildRegisterMetadata() (map[string]string, error) {
	metadata := cloneMetadata(r.cfg.Metadata)
	provider, ok := getMetadataProvider(r.cfg.ServiceName)
	if !ok {
		if err := validateRegisterMetadata(metadata); err != nil {
			return nil, fmt.Errorf("validate register metadata for service %q: %w", r.cfg.ServiceName, err)
		}
		return metadata, nil
	}
	for _, key := range reservedAgentMetadataKeys {
		if _, exists := metadata[key]; exists {
			return nil, fmt.Errorf(
				"static register metadata for service %q contains reserved agent key %q while a metadata provider is registered",
				r.cfg.ServiceName, key)
		}
	}
	dynamic, err := provider.Metadata()
	if err != nil {
		return nil, fmt.Errorf("metadata provider for service %q failed: %w", r.cfg.ServiceName, err)
	}
	if metadata == nil && len(dynamic) > 0 {
		metadata = make(map[string]string, len(dynamic))
	}
	for key, value := range dynamic {
		metadata[key] = value
	}
	if err := validateRegisterMetadata(metadata); err != nil {
		return nil, fmt.Errorf("validate register metadata for service %q: %w", r.cfg.ServiceName, err)
	}
	return metadata, nil
}

// SDKContext returns the Polaris SDK context backing this registry.
func (r *Registry) SDKContext() api.SDKContext {
	if r == nil {
		return nil
	}
	if r.sdkCtx != nil {
		return r.sdkCtx
	}
	if r.Provider != nil {
		return r.Provider.SDKContext()
	}
	return nil
}

// Version returns the instance version configured for this registry.
func (r *Registry) Version() string {
	if r == nil || r.cfg == nil {
		return ""
	}
	return r.cfg.Version
}

// FromFramework returns the Polaris registry bound to service by tRPC-Go.
func FromFramework(service string) (*Registry, bool) {
	r, ok := registry.Get(service).(*Registry)
	if !ok || r == nil {
		return nil, false
	}
	return r, true
}

func (r *Registry) heartBeats() {
	frozenTicker := &time.Ticker{} // waiting on nil frozenTicker.C blocks forever.
	ticker := frozenTicker
	newTicker := func() *time.Ticker {
		r.heartBeat()
		return time.NewTicker(time.Second * time.Duration(r.cfg.HeartBeat))
	}

	select {
	case status := <-r.statuses:
		if status == healthcheck.Serving {
			ticker = newTicker()
		} else {
			// otherwise, service is not ready to serve and ticker should keep frozen.
			plog.GetBaseLogger().Debugf(
				"heartbeat is delayed until the status of service %s is changed to serving",
				r.cfg.ServiceName)
		}
	default:
		// service is not registered to healthcheck, start heart beat immediately.
		ticker = newTicker()
	}

	for {
		select {
		case <-ticker.C:
			r.heartBeat()
		case status := <-r.statuses:
			if status != healthcheck.Serving && ticker != frozenTicker {
				plog.GetBaseLogger().Errorf(
					"heartbeat stopped since the status of service %s is changed to %v",
					r.cfg.ServiceName, status)
				ticker.Stop()
				ticker = frozenTicker
			} else if status == healthcheck.Serving && ticker == frozenTicker {
				ticker = newTicker()
			}
		}
	}
}

func (r *Registry) heartBeat() {
	heartBeatRequest := &api.InstanceHeartbeatRequest{
		InstanceHeartbeatRequest: model.InstanceHeartbeatRequest{
			Service:      r.cfg.ServiceName,
			ServiceToken: r.cfg.ServiceToken,
			Namespace:    r.cfg.Namespace,
			InstanceID:   r.cfg.InstanceID,
			Host:         r.host,
			Port:         r.port,
		},
	}
	if err := r.Provider.Heartbeat(heartBeatRequest); err != nil {
		plog.GetBaseLogger().Errorf("heartbeat report err: %v\n", err)
		metrics.ReportHeartBeatFail(heartBeatRequest)
	} else {
		plog.GetBaseLogger().Debugf("heart beat success")
	}
}

// Deregister anti-registration.
func (r *Registry) Deregister(_ string) error {
	if !r.cfg.EnableRegister {
		return nil
	}
	req := &api.InstanceDeRegisterRequest{
		InstanceDeRegisterRequest: model.InstanceDeRegisterRequest{
			Service:      r.cfg.ServiceName,
			Namespace:    r.cfg.Namespace,
			InstanceID:   r.cfg.InstanceID,
			ServiceToken: r.cfg.ServiceToken,
			Host:         r.host,
			Port:         r.port,
		},
	}
	if err := r.Provider.Deregister(req); err != nil {
		return fmt.Errorf("deregister error: %s", err.Error())
	}
	return nil
}
