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

package selector

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/codec"
	thttp "trpc.group/trpc-go/trpc-go/http"
	"trpc.group/trpc-go/trpc-go/naming/registry"
	"trpc.group/trpc-go/trpc-go/naming/selector"
	"trpc.group/trpc-go/trpc-naming-polarismesh/internal/polaris"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_api"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_loadbalancer"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_model"
	"trpc.group/trpc-go/trpc-naming-polarismesh/mock/mock_plugin"

	"github.com/golang/mock/gomock"
	"github.com/polarismesh/polaris-go/pkg/config"
	"github.com/polarismesh/polaris-go/pkg/model"
	"github.com/polarismesh/polaris-go/pkg/model/local"
	"github.com/polarismesh/polaris-go/pkg/model/pb"
	"github.com/polarismesh/polaris-go/pkg/sdk"
	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestIsInstanceNotFoundHandlesWrappedSDKError(t *testing.T) {
	sdkErr := model.NewSDKError(model.ErrCodeAPIInstanceNotFound, nil, "instance not found")
	require.True(t, isInstanceNotFound(fmt.Errorf("wrapped: %w", sdkErr)))
	require.False(t, isInstanceNotFound(fmt.Errorf("wrapped: %w", errors.New("other error"))))
}

func TestSetup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := mock_api.NewMockSDKContext(ctrl)

	err := Setup(m, &Config{})
	assert.Nil(t, err)
}

func TestNew(t *testing.T) {
	_, err := New(&Config{
		RefreshInterval: 1000,
		ServerAddrs:     []string{"not_exist"},
	})
	assert.Nil(t, err)
}

func TestExtractSourceServiceRequestInfo(t *testing.T) {
	testOptions := []*selector.Options{
		// metadata and hosting service.
		{
			SourceMetadata: map[string]string{
				"a": "b",
			},
			SourceEnvName:     "source env",
			SourceServiceName: "service name",
		},
		// Hosting service only.
		{
			SourceServiceName: "service name",
		},
		// Only the calling namespace.
		{
			SourceNamespace: "test",
		},
		// Only metadata.
		{
			SourceMetadata: map[string]string{
				"a": "b",
			},
			SourceEnvName: "source env",
		},
		// metadata and calling service neither.
		{},
	}

	info := extractSourceServiceRequestInfo(testOptions[0], false)
	assert.Equal(t, info.Service, "service name")
	assert.Equal(t, info.Metadata["a"], "b")
	assert.Equal(t, info.Metadata["env"], "source env")

	info = extractSourceServiceRequestInfo(testOptions[1], false)
	assert.Equal(t, info.Service, "service name")
	assert.Equal(t, info.Namespace, "")
	assert.Equal(t, len(info.Metadata), 0)

	info = extractSourceServiceRequestInfo(testOptions[2], false)
	assert.Equal(t, info.Service, "")
	assert.Equal(t, info.Namespace, "test")
	assert.Equal(t, len(info.Metadata), 0)

	info = extractSourceServiceRequestInfo(testOptions[3], false)
	assert.Equal(t, info.Service, "")
	assert.Equal(t, info.Metadata["a"], "b")
	assert.Equal(t, info.Metadata["env"], "source env")

	info = extractSourceServiceRequestInfo(testOptions[4], false)
	assert.Nil(t, info)
}

func TestSelect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("host").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(1003)).AnyTimes()
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	inst.EXPECT().GetCircuitBreakerStatus().Return(nil).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetEngine().Return(&permissionEngine{
		checkResult: &model.CheckResult{Pass: true},
	}).AnyTimes()
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().GetOneInstance(gomock.Any()).Return(&model.OneInstanceResponse{
		InstancesResponse: model.InstancesResponse{
			Instances: []model.Instance{inst},
		},
	}, nil).AnyTimes()
	s := &Selector{
		consumer: consumer,
		cfg:      &Config{},
	}

	node, err := s.Select("service name")
	assert.Nil(t, err)
	assert.Equal(t, node.Weight, 100)
}

func TestSelectSkipsExhaustedHalfOpenNode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "half-open-selector"}
	newInstance := func(id, host string, port uint32, status model.CircuitBreakerStatus) model.Instance {
		localValue := local.NewInstanceLocalValue().(*local.DefaultInstanceLocalValue)
		if status != nil {
			localValue.SetCircuitBreakerStatus(status)
		}
		return pb.NewInstanceInProto(&apiservice.Instance{
			Id:      wrapperspb.String(id),
			Host:    wrapperspb.String(host),
			Port:    wrapperspb.UInt32(port),
			Weight:  wrapperspb.UInt32(100),
			Healthy: wrapperspb.Bool(true),
		}, svcKey, localValue)
	}
	halfOpen := model.NewHalfOpenStatus("selector-probe", time.Now(), 1).(*model.HalfOpenStatus)
	require.True(t, halfOpen.AcquirePermission())
	halfOpenInst := newInstance("half-open", "127.0.0.1", 8000, halfOpen)
	healthyInst := newInstance("healthy", "127.0.0.1", 8001, nil)
	instances := model.NewDefaultServiceInstances(
		model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		[]model.Instance{halfOpenInst, healthyInst},
	)
	cluster := model.NewCluster(instances.GetServiceClusters(), nil)
	clusterInstances, _ := cluster.GetInstances()
	require.Len(t, clusterInstances, 2)

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	engine := &permissionEngine{checkResult: &model.CheckResult{Pass: false}}
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetEngine().Return(engine).AnyTimes()

	lbPlugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	lbPlugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).
		Return(healthyInst, nil).Times(1)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().
		GetPlugin(gomock.Any(), gomock.Any()).Return(lbPlugin, nil).Times(1)
	sdkCtx.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().GetOneInstance(gomock.Any()).Return(&model.OneInstanceResponse{
		InstancesResponse: model.InstancesResponse{Instances: []model.Instance{halfOpenInst}},
	}, nil).Times(1)
	consumer.EXPECT().GetInstances(gomock.Any()).Return(&model.InstancesResponse{
		ServiceInfo: model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		Cluster:     cluster,
		Instances:   []model.Instance{halfOpenInst, healthyInst},
	}, nil).Times(1)

	selector := &Selector{consumer: consumer, cfg: &Config{}}
	node, err := selector.Select(svcKey.Service)
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:8001", node.Address)
}

func TestSelectRebuildsMethodResourceAfterInstanceFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	svcKey := &model.ServiceKey{Namespace: "default", Service: "method-fallback"}
	newInstance := func(protocol string, port uint32) model.Instance {
		return pb.NewInstanceInProto(&apiservice.Instance{
			Id:       wrapperspb.String(protocol),
			Host:     wrapperspb.String("127.0.0.1"),
			Port:     wrapperspb.UInt32(port),
			Protocol: wrapperspb.String(protocol),
			Weight:   wrapperspb.UInt32(100),
			Healthy:  wrapperspb.Bool(true),
		}, svcKey, local.NewInstanceLocalValue())
	}
	grpcInst := newInstance("grpc", 8000)
	httpInst := newInstance("http", 8001)
	instances := model.NewDefaultServiceInstances(
		model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		[]model.Instance{grpcInst, httpInst},
	)
	cluster := model.NewCluster(instances.GetServiceClusters(), nil)

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	engine := &permissionEngine{checkResult: &model.CheckResult{Pass: true}}
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetEngine().Return(engine).AnyTimes()

	lbPlugin := mock_loadbalancer.NewMockLoadBalancer(ctrl)
	lbPlugin.EXPECT().ChooseInstance(gomock.Any(), gomock.Any()).
		Return(httpInst, nil).Times(1)
	pluginManager := mock_plugin.NewMockManager(ctrl)
	pluginManager.EXPECT().GetPlugin(gomock.Any(), gomock.Any()).
		Return(lbPlugin, nil).Times(1)
	sdkCtx.EXPECT().GetPlugins().Return(pluginManager).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().GetOneInstance(gomock.Any()).Return(&model.OneInstanceResponse{
		InstancesResponse: model.InstancesResponse{Instances: []model.Instance{grpcInst}},
	}, nil).Times(1)
	consumer.EXPECT().GetInstances(gomock.Any()).Return(&model.InstancesResponse{
		ServiceInfo: model.ServiceInfo{Service: svcKey.Service, Namespace: svcKey.Namespace},
		Cluster:     cluster,
		Instances:   []model.Instance{grpcInst, httpInst},
	}, nil).Times(1)

	msg := trpc.Message(context.Background())
	msg.WithClientRPCName("/review.Service/Call")
	requestCtx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)
	sel := &Selector{consumer: consumer, cfg: &Config{Enable: true}}
	node, err := sel.Select(svcKey.Service, selector.WithContext(requestCtx))
	require.NoError(t, err)

	methodResource, ok := node.Metadata[polaris.MethodResourceMetadataKey].(*model.MethodResource)
	require.True(t, ok)
	require.Equal(t, "http", methodResource.Protocol)
	require.Len(t, engine.methodResource, 2)
	require.Equal(t, "grpc", engine.methodResource[0].(*model.MethodResource).Protocol)
	require.Equal(t, "http", engine.methodResource[1].(*model.MethodResource).Protocol)
}

func TestReport(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	inst.EXPECT().GetHost().Return("127.0.0.1").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(8000)).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	engine := &reportEngine{}
	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetEngine().Return(engine).AnyTimes()
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).AnyTimes()
	s := &Selector{
		consumer: consumer,
		cfg:      &Config{},
	}

	node := &registry.Node{
		Metadata: map[string]interface{}{
			"instance": inst,
		},
	}

	assert.Nil(t, s.Report(node, time.Second, nil))
	assert.NotNil(t, engine.stat)
}

func TestSelectAndReportPreserveCallerResource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	inst := mock_model.NewMockInstance(ctrl)
	inst.EXPECT().GetMetadata().Return(map[string]string{}).AnyTimes()
	inst.EXPECT().GetWeight().Return(100).AnyTimes()
	inst.EXPECT().GetHost().Return("127.0.0.1").AnyTimes()
	inst.EXPECT().GetPort().Return(uint32(8000)).AnyTimes()
	inst.EXPECT().GetNamespace().Return("default").AnyTimes()
	inst.EXPECT().GetService().Return("review-service").AnyTimes()
	inst.EXPECT().GetProtocol().Return("grpc").AnyTimes()
	inst.EXPECT().GetCircuitBreakerStatus().Return(model.NewCircuitBreakerStatus(
		"caller-rule", model.HalfOpen, time.Now())).AnyTimes()

	sdkConfig := config.NewDefaultConfiguration([]string{"127.0.0.1:0"})
	sdkConfig.GetConsumer().GetCircuitBreaker().SetEnable(true)
	engine := &callerEngine{checkResult: &model.CheckResult{Pass: true}}
	sdkCtx := mock_api.NewMockSDKContext(ctrl)
	sdkCtx.EXPECT().GetConfig().Return(sdkConfig).AnyTimes()
	sdkCtx.EXPECT().GetEngine().Return(engine).AnyTimes()

	consumer := mock_api.NewMockConsumerAPI(ctrl)
	consumer.EXPECT().SDKContext().Return(sdkCtx).AnyTimes()
	consumer.EXPECT().GetOneInstance(gomock.Any()).Return(&model.OneInstanceResponse{
		InstancesResponse: model.InstancesResponse{Instances: []model.Instance{inst}},
	}, nil).Times(1)
	consumer.EXPECT().UpdateServiceCallResult(gomock.Any()).Return(nil).Times(1)

	s := &Selector{consumer: consumer, cfg: &Config{Enable: true}}
	node, err := s.Select("review-service",
		selector.WithSourceNamespace("callers"),
		selector.WithSourceServiceName("orders"))
	require.NoError(t, err)

	wantCaller := &model.ServiceKey{Namespace: "callers", Service: "orders"}
	require.NotNil(t, engine.checkResource)
	require.Equal(t, wantCaller, engine.checkResource.GetCallerService())
	require.NoError(t, s.Report(node, time.Second, nil))
	require.NotNil(t, engine.reportStat)
	require.Equal(t, wantCaller, engine.reportStat.Resource.GetCallerService())
}

type reportEngine struct {
	sdk.Engine
	stat *model.ResourceStat
}

type permissionEngine struct {
	sdk.Engine
	checkResult    *model.CheckResult
	methodResource []model.Resource
}

func (e *permissionEngine) Check(resource model.Resource) (*model.CheckResult, error) {
	switch res := resource.(type) {
	case *model.ServiceResource:
		return &model.CheckResult{Pass: true}, nil
	case *model.InstanceResource:
		return &model.CheckResult{Pass: res.GetNode().Port != 8000}, nil
	case *model.MethodResource:
		e.methodResource = append(e.methodResource, res)
		return &model.CheckResult{Pass: true}, nil
	}
	return e.checkResult, nil
}

func (e *reportEngine) Report(stat *model.ResourceStat) error {
	e.stat = stat
	return nil
}

type callerEngine struct {
	sdk.Engine
	checkResult   *model.CheckResult
	checkResource model.Resource
	reportStat    *model.ResourceStat
}

func (e *callerEngine) Check(resource model.Resource) (*model.CheckResult, error) {
	e.checkResource = resource
	return e.checkResult, nil
}

func (e *callerEngine) Report(stat *model.ResourceStat) error {
	e.reportStat = stat
	return nil
}

func TestSetTransSelectorMeta(t *testing.T) {
	ctx := context.Background()
	msg := trpc.Message(ctx)
	msg.WithServerMetaData(codec.MetaData{
		"selector-meta-k": []byte("v"),
		"k2":              []byte("v2"),
	})
	ctx = context.WithValue(ctx, codec.ContextKeyMessage, msg)
	opts := &selector.Options{Ctx: ctx}
	selectorMeta := make(map[string]string, 0)

	setTransSelectorMeta(opts, selectorMeta)
	assert.EqualValues(t, "v", selectorMeta["k"])
	assert.EqualValues(t, "", selectorMeta["k2"])
	opts.Ctx = nil
	selectorMeta = make(map[string]string, 0)
	setTransSelectorMeta(opts, selectorMeta)
	assert.EqualValues(t, 0, len(selectorMeta))
}

func TestRequestHTTPMethod(t *testing.T) {
	msg := trpc.Message(context.Background())
	msg.WithClientReqHead(&thttp.ClientReqHeader{Method: "GET"})
	ctx := context.WithValue(context.Background(), codec.ContextKeyMessage, msg)
	require.Equal(t, "GET", requestHTTPMethod(&selector.Options{Ctx: ctx}))
	require.Empty(t, requestHTTPMethod(&selector.Options{}))
}
