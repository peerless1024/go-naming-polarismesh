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
	"errors"
	"fmt"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/healthcheck"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/plugin"

	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_api"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/api"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFactoryRegistry(t *testing.T) {
	c := gomock.NewController(t)
	defer c.Finish()
	err := register(
		mock_api.NewMockProviderAPI(c),
		nil,
		&FactoryConfig{
			Services: []Service{
				{
					Token: "token",
				},
			},
		})

	assert.Nil(t, err)
}

func TestSetup(t *testing.T) {
	factory := &RegistryFactory{}
	assert.Equal(t, factory.Type(), "registry")
	assert.NotNil(t, factory.Setup("registry", nil))
}

func TestConfigSetup(t *testing.T) {
	cfgstr := `
plugins:
  registry:
    polarismesh:
      connect_timeout: 1000
      message_timeout: 1s
      join_point: default
      disable_health_check: true
      address_list: "not_exist"
`
	cfg := trpc.Config{}
	err := yaml.Unmarshal([]byte(cfgstr), &cfg)
	assert.Nil(t, err)
	polarisCfg := cfg.Plugins["registry"]["polarismesh"]
	pluginFac := &RegistryFactory{}
	err = pluginFac.Setup("polarismesh", &polarisCfg)
	assert.Nil(t, err)
	require.NotNil(t, pluginFac.GetSDKCtx())
}

func TestNew(t *testing.T) {
	_, err := newRegistry(nil, &Config{})
	require.Nil(t, err)
}

func TestRegisterDeRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	resp := &model.InstanceRegisterResponse{}
	p := mock_api.NewMockProviderAPI(ctrl)
	p.EXPECT().Register(gomock.Any()).Return(resp, nil).AnyTimes()
	p.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
	p.EXPECT().Deregister(gomock.Any()).Return(nil).AnyTimes()
	r := &Registry{
		cfg: &Config{
			EnableRegister: true,
			BindAddress:    "lo:8080",
			HeartBeat:      1,
		},
		Provider: p,
	}
	err := r.Register("")
	assert.Nil(t, err)

	err = r.Deregister("")
	assert.Nil(t, err)
}

func TestFlexDepends(t *testing.T) {
	require.Equal(t, []string{"selector-polarismesh"}, (&RegistryFactory{}).FlexDependsOn())
}

func TestRegistryInstanceLocation(t *testing.T) {
	const region, zone, campus = "China", "Guangzhou", "Shenzhen"

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	newProviderAPIByContext := api.NewProviderAPIByContext
	api.NewProviderAPIByContext = func(_ api.SDKContext) api.ProviderAPI {
		mocked := mock_api.NewMockProviderAPI(ctrl)
		mocked.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Register(gomock.Not(gomock.Nil())).DoAndReturn(
			func(instance *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.Equal(t, region, instance.Location.Region)
				require.Equal(t, zone, instance.Location.Zone)
				require.Equal(t, campus, instance.Location.Campus)
				require.NotNil(t, instance.Version)
				require.Equal(t, "v1.2.3", *instance.Version)
				return &model.InstanceRegisterResponse{InstanceID: "instance_id"}, nil
			}).Times(1)
		return mocked
	}
	defer func() { api.NewProviderAPIByContext = newProviderAPIByContext }()

	node := yaml.Node{}
	require.Nil(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
register_self: true
instance_location:
  region: %s
  zone: %s
  campus: %s
address_list: "not_exist"
service:
  - name: %s
    token: "xxx"
    namespace: Development
    version: v1.2.3
    bind_address: 127.0.0.1:8080
`, region, zone, campus, t.Name())), &node))
	require.Nil(t, (&RegistryFactory{}).Setup("polarismesh", &plugin.YamlNodeDecoder{Node: &node}))

	r := registry.Get(t.Name())
	require.NotNil(t, r)
	require.Nil(t, r.Register(""))
}

func TestHealthCheckHeartbeat(t *testing.T) {
	t.Run("service unregistered to healthcheck heart beat immediately", func(t *testing.T) {
		heartbeat := make(chan struct{})
		r, err := NewRegistry(&provider{heartbeat: heartbeat}, &Config{
			ServiceName:  "service",
			HeartBeat:    10,
			ServiceToken: "token",
		})
		require.Nil(t, err)
		require.Nil(t, r.Register(""))
		select {
		case <-time.After(time.Second):
			require.FailNow(t, "heartbeat must be called immediately")
		case <-heartbeat:
		}
	})
	t.Run("service registered to healthcheck heart beat on serving", func(t *testing.T) {
		heartbeat := make(chan struct{})
		r, err := NewRegistry(&provider{heartbeat: heartbeat}, &Config{
			ServiceName:  "service",
			HeartBeat:    1,
			ServiceToken: "token",
		})
		require.Nil(t, err)

		hc := healthcheck.New(healthcheck.WithStatusWatchers(healthcheck.GetWatchers()))
		update, err := hc.Register("service")
		require.Nil(t, err)
		require.Nil(t, r.Register(""))

		select {
		case <-time.After(time.Second * 2):
		case <-heartbeat:
			require.FailNow(t, "the first heartbeat should not start before service serving")
		}

		update(healthcheck.Serving)
		select {
		case <-time.After(time.Second):
			require.FailNow(t, "heartbeat should start immediately when service start serving")
		case <-heartbeat:
		}
	})
}

func TestRegistryZeroWeight(t *testing.T) {
	const weight = 0
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	newProviderAPIByContext := api.NewProviderAPIByContext
	api.NewProviderAPIByContext = func(_ api.SDKContext) api.ProviderAPI {
		mocked := mock_api.NewMockProviderAPI(ctrl)
		mocked.EXPECT().Heartbeat(gomock.Any()).Return(nil).AnyTimes()
		mocked.EXPECT().Register(gomock.Not(gomock.Nil())).DoAndReturn(
			func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
				require.NotNil(t, req.Weight)
				require.Equal(t, weight, *req.Weight)
				return &model.InstanceRegisterResponse{}, nil
			}).Times(1)
		return mocked
	}
	defer func() { api.NewProviderAPIByContext = newProviderAPIByContext }()

	node := yaml.Node{}
	require.Nil(t, yaml.Unmarshal([]byte(fmt.Sprintf(`
register_self: true
heartbeat_interval: 3000
protocol: grpc
address_list: "not_exist"
service:                         
  - name: %s
    namespace: Development       
    token: xxxxxxxxxxxxxxxxxxx   
    instance_id: xxxxxxxxxxxxxxxx
    weight: %d
    bind_address: 127.0.0.1:8080
`, t.Name(), weight)), &node))
	require.Nil(t, (&RegistryFactory{}).Setup("polarismesh", &plugin.YamlNodeDecoder{Node: &node}))
	r := registry.Get(t.Name())
	require.Nil(t, r.Register(t.Name()))
}

func TestRegisterMergesMetadataAndVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"env":           "dynamic",
			"ai-agent-name": "customer-agent",
		}, nil
	})))
	defer UnregisterMetadataProvider(service)

	providerAPI := mock_api.NewMockProviderAPI(ctrl)
	providerAPI.EXPECT().Register(gomock.Any()).DoAndReturn(
		func(req *api.InstanceRegisterRequest) (*model.InstanceRegisterResponse, error) {
			require.Equal(t, "dynamic", req.Metadata["env"])
			require.Equal(t, "orders", req.Metadata["business"])
			require.Equal(t, "customer-agent", req.Metadata["ai-agent-name"])
			require.NotNil(t, req.Version)
			require.Equal(t, "v2.0.0", *req.Version)
			return &model.InstanceRegisterResponse{InstanceID: "instance-id"}, nil
		}).Times(1)

	static := map[string]string{"env": "static", "business": "orders"}
	r := &Registry{
		cfg: &Config{
			EnableRegister: true,
			ServiceName:    service,
			Version:        "v2.0.0",
			Metadata:       static,
		},
		Provider: providerAPI,
	}
	require.NoError(t, r.register())
	require.Equal(t, "static", static["env"])
}

func TestRegisterProviderFailurePreventsRegistrationAndHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	service := t.Name()
	providerErr := errors.New("agent card unavailable")
	require.NoError(t, RegisterMetadataProvider(service, MetadataProviderFunc(func() (map[string]string, error) {
		return nil, providerErr
	})))
	defer UnregisterMetadataProvider(service)

	providerAPI := mock_api.NewMockProviderAPI(ctrl)
	r := &Registry{
		cfg: &Config{
			EnableRegister: true,
			ServiceName:    service,
			BindAddress:    "127.0.0.1:8080",
		},
		Provider: providerAPI,
		statuses: make(chan healthcheck.Status, 1),
	}
	err := r.Register("")
	require.ErrorIs(t, err, providerErr)
}

func TestRegistrySDKContextAndFromFramework(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	r := &Registry{sdkCtx: sdkCtx, cfg: &Config{Version: "1.2.3"}}
	require.Same(t, sdkCtx, r.SDKContext())
	require.Equal(t, "1.2.3", r.Version())

	service := t.Name()
	registry.Register(service, r)
	got, ok := FromFramework(service)
	require.True(t, ok)
	require.Same(t, r, got)
	require.Same(t, sdkCtx, got.SDKContext())

	missing, ok := FromFramework(service + "-missing")
	require.False(t, ok)
	require.Nil(t, missing)

	otherService := service + "-other"
	registry.Register(otherService, &registry.NoopRegistry{})
	other, ok := FromFramework(otherService)
	require.False(t, ok)
	require.Nil(t, other)
	require.Nil(t, (*Registry)(nil).SDKContext())
}

type provider struct {
	api.ProviderAPI
	heartbeat chan struct{}
}

func (p *provider) Heartbeat(*api.InstanceHeartbeatRequest) error {
	p.heartbeat <- struct{}{}
	return nil
}
