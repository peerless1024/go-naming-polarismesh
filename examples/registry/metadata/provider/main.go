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

// Package main demonstrates dynamic metadata registration through Polaris.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strconv"

	"trpc.group/trpc-go/trpc-go"
	"trpc.group/trpc-go/trpc-go/server"
	_ "trpc.group/trpc-go/trpc-naming-polarismesh"
	"trpc.group/trpc-go/trpc-naming-polarismesh/examples/internal/localip"
	polarisregistry "trpc.group/trpc-go/trpc-naming-polarismesh/registry"

	"github.com/polarismesh/polaris-go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	defaultNamespace = "go-naming-polarismesh-e2e"
	defaultService   = "trpc.polaris.registry.metadata.Echo"
)

var (
	configPath    string
	polarisServer string
	polarisPort   int
	namespace     string
	serviceName   string
	version       string
	metadataMode  string
	listenPort    int
	debug         bool
)

type echoService interface {
	Echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

type echoServiceImpl struct{}

func (s *echoServiceImpl) Echo(
	_ context.Context,
	req *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return wrapperspb.String("registry metadata: " + req.GetValue()), nil
}

func echoHandler(
	svr interface{},
	ctx context.Context,
	filterFactory server.FilterFunc,
) (interface{}, error) {
	req := &wrapperspb.StringValue{}
	filters, err := filterFactory(req)
	if err != nil {
		return nil, err
	}
	handle := func(ctx context.Context, reqBody interface{}) (interface{}, error) {
		return svr.(echoService).Echo(ctx, reqBody.(*wrapperspb.StringValue))
	}
	return filters.Filter(ctx, req, handle)
}

func main() {
	initArgs()
	flag.Parse()
	if polarisServer == "" {
		log.Fatal("--polaris-server or POLARIS_SERVER is required")
	}
	switch metadataMode {
	case "dynamic", "none", "conflict":
	default:
		log.Fatalf("unsupported metadata mode %q", metadataMode)
	}
	localHost, err := localip.FromServer(polarisServer, polarisPort)
	if err != nil {
		log.Fatalf("resolve local host from Polaris connection: %v", err)
	}
	setEnvironment(localHost)
	trpc.ServerConfigPath = configPath

	if debug {
		if err := api.SetLoggersLevel(api.DebugLog); err != nil {
			log.Printf("[WARN] enable Polaris debug logs: %v", err)
		}
	}
	if metadataMode != "none" {
		provider, err := dynamicMetadata()
		if err != nil {
			log.Fatalf("build metadata provider: %v", err)
		}
		if err := polarisregistry.RegisterMetadataProvider(serviceName, provider); err != nil {
			log.Fatalf("register metadata provider: %v", err)
		}
		defer polarisregistry.UnregisterMetadataProvider(serviceName)
	}

	s := trpc.NewServer()
	service := s.Service(serviceName)
	if service == nil {
		log.Fatalf("service %s is not configured", serviceName)
	}
	serviceDesc := &server.ServiceDesc{
		ServiceName: serviceName,
		HandlerType: (*echoService)(nil),
		Methods: []server.Method{{
			Name: "/" + serviceName + "/Echo",
			Func: echoHandler,
		}},
	}
	if err := service.Register(serviceDesc, &echoServiceImpl{}); err != nil {
		log.Fatalf("register echo service: %v", err)
	}

	log.Printf("dynamic metadata provider listening on %s:%d as %s",
		localHost, listenPort, serviceName)
	if err := s.Serve(); err != nil {
		log.Fatalf("serve registry metadata example: %v", err)
	}
}

func dynamicMetadata() (polarisregistry.MetadataProvider, error) {
	card, err := json.Marshal(map[string]string{
		"name":        "dynamic-registry-agent",
		"version":     version,
		"description": "dynamic metadata registry example",
	})
	if err != nil {
		return nil, err
	}
	return polarisregistry.MetadataProviderFunc(func() (map[string]string, error) {
		return map[string]string{
			"ai-agent-name":    "dynamic-registry-agent",
			"a2a.agent.card":   string(card),
			"a2a.transport":    "JSONRPC",
			"a2a.path":         "/a2a",
			"registry.runtime": "dynamic",
			"registry.version": version,
		}, nil
	}), nil
}

func initArgs() {
	flag.StringVar(&configPath, "conf", "trpc_go.yaml", "tRPC configuration path")
	flag.StringVar(&polarisServer, "polaris-server", os.Getenv("POLARIS_SERVER"),
		"Polaris host without scheme")
	flag.IntVar(&polarisPort, "polaris-port", envInt("POLARIS_GRPC_PORT", 8091),
		"Polaris gRPC port")
	flag.StringVar(&namespace, "namespace", envDefault("POLARIS_NAMESPACE", defaultNamespace),
		"Polaris namespace")
	flag.StringVar(&serviceName, "service", envDefault("POLARIS_SERVICE", defaultService),
		"Polaris service name")
	flag.StringVar(&version, "version", envDefault("POLARIS_VERSION", "v1.2.3"),
		"registered instance version")
	flag.StringVar(&metadataMode, "metadata-mode", envDefault("METADATA_MODE", "dynamic"),
		"metadata mode: dynamic, none or conflict")
	flag.IntVar(&listenPort, "port", envInt("PROVIDER_PORT", 28271),
		"provider listen port")
	flag.BoolVar(&debug, "debug", false, "enable Polaris debug logs")
}

func setEnvironment(localHost string) {
	setenv("POLARIS_SERVER", polarisServer)
	setenv("POLARIS_GRPC_PORT", strconv.Itoa(polarisPort))
	setenv("POLARIS_NAMESPACE", namespace)
	setenv("POLARIS_SERVICE", serviceName)
	setenv("POLARIS_VERSION", version)
	setenv("PROVIDER_HOST", localHost)
	setenv("PROVIDER_PORT", strconv.Itoa(listenPort))
}

func setenv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		log.Fatalf("set %s: %v", key, err)
	}
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := envDefault(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err == nil {
		return value
	}
	return fallback
}
