package bundle

import (
	"os"

	"context"

	"net/http"

	"net"

	"github.com/fission/fission-workflows/pkg/api/function"
	"github.com/fission/fission-workflows/pkg/api/invocation"
	"github.com/fission/fission-workflows/pkg/api/workflow"
	"github.com/fission/fission-workflows/pkg/api/workflow/parse"
	"github.com/fission/fission-workflows/pkg/apiserver"
	"github.com/fission/fission-workflows/pkg/controller"
	"github.com/fission/fission-workflows/pkg/controller/expr"
	"github.com/fission/fission-workflows/pkg/fes"
	"github.com/fission/fission-workflows/pkg/fes/eventstore/nats"
	"github.com/fission/fission-workflows/pkg/fnenv"
	"github.com/fission/fission-workflows/pkg/fnenv/fission"
	"github.com/fission/fission-workflows/pkg/fnenv/native"
	"github.com/fission/fission-workflows/pkg/fnenv/native/builtin"
	"github.com/fission/fission-workflows/pkg/scheduler"
	"github.com/fission/fission-workflows/pkg/types/aggregates"
	"github.com/fission/fission-workflows/pkg/types/typedvalues"
	"github.com/fission/fission-workflows/pkg/util"
	"github.com/fission/fission-workflows/pkg/util/labels"
	"github.com/fission/fission-workflows/pkg/util/pubsub"
	"github.com/fission/fission-workflows/pkg/version"
	controllerc "github.com/fission/fission/controller/client"
	executor "github.com/fission/fission/executor/client"
	"github.com/gorilla/handlers"
	grpcruntime "github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/nats-io/go-nats-streaming"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

const (
	gRPCAddress         = ":5555"
	apiGatewayAddress   = ":8080"
	fissionProxyAddress = ":8888"
)

type Options struct {
	Nats                  *NatsOptions
	Fission               *FissionOptions
	InternalRuntime       bool
	InvocationController  bool
	WorkflowController    bool
	ApiAdmin              bool
	ApiWorkflow           bool
	ApiHttp               bool
	ApiWorkflowInvocation bool
}

type FissionOptions struct {
	ExecutorAddress string
	ControllerAddr  string
}

type NatsOptions struct {
	Url     string
	Client  string
	Cluster string
}

// Run serves enabled components in a blocking way
func Run(ctx context.Context, opts *Options) error {
	log.WithField("version", version.VERSION).Info("Starting bundle...")

	var es fes.EventStore
	var esPub pubsub.Publisher

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()

	// Event Stores
	if opts.Nats != nil {
		log.WithFields(log.Fields{
			"url":     opts.Nats.Url,
			"cluster": opts.Nats.Cluster,
			"client":  opts.Nats.Client,
		}).Infof("Using event store: NATS")
		natsEs := setupNatsEventStoreClient(opts.Nats.Url, opts.Nats.Cluster, opts.Nats.Client)
		es = natsEs
		esPub = natsEs
	}
	if es == nil {
		panic("no event store provided")
	}

	// Caches
	wfiCache := getWorkflowInvocationCache(ctx, esPub)
	wfCache := getWorkflowCache(ctx, esPub)

	// Resolvers and runtimes
	resolvers := map[string]fnenv.Resolver{}
	runtimes := map[string]fnenv.Runtime{}
	if opts.InternalRuntime {
		log.WithField("config", nil).Infof("Using Function Runtime: Internal")
		runtimes["internal"] = setupInternalFunctionRuntime()
		resolvers["internal"] = setupInternalFunctionRuntime()
	}
	if opts.Fission != nil {
		log.WithFields(log.Fields{
			"controller": opts.Fission.ControllerAddr,
			"executor":   opts.Fission.ExecutorAddress,
		}).Infof("Using Function Runtime: Fission")
		runtimes["fission"] = setupFissionFunctionRuntime(opts.Fission.ExecutorAddress)
		resolvers["fission"] = setupFissionFunctionResolver(opts.Fission.ControllerAddr)
	}

	// Controller
	if opts.InvocationController || opts.WorkflowController {
		var ctrls []controller.Controller
		if opts.InvocationController {
			log.Info("Using controller: workflow")
			ctrls = append(ctrls, setupWorkflowController(wfCache(), es, resolvers))
		}

		if opts.WorkflowController {
			log.Info("Using controller: invocation")
			ctrls = append(ctrls, setupInvocationController(wfiCache(), wfCache(), es, runtimes))
		}

		log.Info("Running controllers.")
		runController(ctx, ctrls...)
	}

	// Http servers
	if opts.Fission != nil {
		proxySrv := &http.Server{Addr: fissionProxyAddress}
		defer proxySrv.Shutdown(ctx)
		runFissionEnvironmentProxy(proxySrv, es, wfiCache(), wfCache(), resolvers)
	}

	if opts.ApiAdmin {
		runAdminApiServer(grpcServer)
	}

	if opts.ApiWorkflow {
		runWorkflowApiServer(grpcServer, es, resolvers, wfCache())
	}

	if opts.ApiWorkflowInvocation {
		runWorkflowInvocationApiServer(grpcServer, es, wfiCache())
	}

	if opts.ApiAdmin || opts.ApiWorkflow || opts.ApiWorkflowInvocation {
		lis, err := net.Listen("tcp", gRPCAddress)
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		defer lis.Close()
		log.Info("Serving gRPC services at: ", lis.Addr())
		go grpcServer.Serve(lis)
	}

	if opts.ApiHttp {
		apiSrv := &http.Server{Addr: apiGatewayAddress}
		defer apiSrv.Shutdown(ctx)
		var admin, wf, wfi string
		if opts.ApiAdmin {
			admin = gRPCAddress
		}
		if opts.ApiWorkflow {
			wf = gRPCAddress
		}
		if opts.ApiWorkflowInvocation {
			wfi = gRPCAddress
		}
		runHttpGateway(ctx, apiSrv, admin, wf, wfi)
	}

	log.Info("Bundle set up.")
	<-ctx.Done()
	log.Info("Shutting down...")
	// TODO properly shutdown components
	return nil
}

func getWorkflowCache(ctx context.Context, eventPub pubsub.Publisher) func() fes.CacheReaderWriter {
	var wfCache fes.CacheReaderWriter
	return func() fes.CacheReaderWriter {
		if wfCache != nil {
			return wfCache
		}

		wfCache = setupWorkflowCache(ctx, eventPub)
		return wfCache
	}
}

func getWorkflowInvocationCache(ctx context.Context, eventPub pubsub.Publisher) func() fes.CacheReaderWriter {
	var wfiCache fes.CacheReaderWriter
	return func() fes.CacheReaderWriter {
		if wfiCache != nil {
			return wfiCache
		}

		wfiCache = setupWorkflowInvocationCache(ctx, eventPub)
		return wfiCache
	}
}

func setupInternalFunctionRuntime() *native.FunctionEnv {
	return native.NewFunctionEnv(builtin.DefaultBuiltinFunctions)
}

func setupFissionFunctionRuntime(executorAddr string) *fission.FunctionEnv {
	client := executor.MakeClient(executorAddr)
	return fission.NewFunctionEnv(client)
}

func setupFissionFunctionResolver(controllerAddr string) *fission.Resolver {
	controllerClient := controllerc.MakeClient(controllerAddr)
	return fission.NewResolver(controllerClient)
}

func setupNatsEventStoreClient(url string, cluster string, clientId string) *nats.EventStore {
	if clientId == "" {
		clientId = util.Uid()
	}

	conn, err := stan.Connect(cluster, clientId, stan.NatsURL(url))
	if err != nil {
		panic(err)
	}

	log.WithField("cluster", cluster).
		WithField("url", "!redacted!").
		WithField("client", clientId).
		Info("connected to NATS")
	es := nats.NewEventStore(nats.NewWildcardConn(conn))
	err = es.Watch(fes.Aggregate{Type: "invocation"})
	if err != nil {
		panic(err)
	}
	err = es.Watch(fes.Aggregate{Type: "workflow"})
	if err != nil {
		panic(err)
	}
	return es
}

func setupWorkflowInvocationCache(ctx context.Context, invocationEventPub pubsub.Publisher) *fes.SubscribedCache {
	invokeSub := invocationEventPub.Subscribe(pubsub.SubscriptionOptions{
		Buf: 50,
		LabelSelector: labels.OrSelector(
			labels.InSelector("aggregate.type", "invocation"),
			labels.InSelector("parent.type", "invocation")),
	})
	wi := func() fes.Aggregator {
		return aggregates.NewWorkflowInvocation("", nil)
	}

	return fes.NewSubscribedCache(ctx, fes.NewMapCache(), wi, invokeSub)
}

func setupWorkflowCache(ctx context.Context, workflowEventPub pubsub.Publisher) *fes.SubscribedCache {
	wfSub := workflowEventPub.Subscribe(pubsub.SubscriptionOptions{
		Buf:           10,
		LabelSelector: labels.InSelector("aggregate.type", "workflow"),
	})
	wb := func() fes.Aggregator {
		return aggregates.NewWorkflow("", nil)
	}
	return fes.NewSubscribedCache(ctx, fes.NewMapCache(), wb, wfSub)
}

func runAdminApiServer(s *grpc.Server) {
	adminServer := &apiserver.GrpcAdminApiServer{}
	apiserver.RegisterAdminAPIServer(s, adminServer)
	log.Infof("Serving admin gRPC API at %s.", gRPCAddress)
}

func runWorkflowApiServer(s *grpc.Server, es fes.EventStore, resolvers map[string]fnenv.Resolver, wfCache fes.CacheReader) {
	workflowParser := parse.NewResolver(resolvers)
	workflowValidator := parse.NewValidator()
	workflowApi := workflow.NewApi(es, workflowParser)
	workflowServer := apiserver.NewGrpcWorkflowApiServer(workflowApi, workflowValidator, wfCache)
	apiserver.RegisterWorkflowAPIServer(s, workflowServer)
	log.Infof("Serving workflow gRPC API at %s.", gRPCAddress)
}

func runWorkflowInvocationApiServer(s *grpc.Server, es fes.EventStore, wfiCache fes.CacheReader) {
	invocationApi := invocation.NewApi(es)
	invocationServer := apiserver.NewGrpcInvocationApiServer(invocationApi, wfiCache)
	apiserver.RegisterWorkflowInvocationAPIServer(s, invocationServer)
	log.Infof("Serving workflow invocation gRPC API at %s.", gRPCAddress)
}

func runHttpGateway(ctx context.Context, gwSrv *http.Server, adminApiAddr string, wfApiAddr string, wfiApiAddr string) {
	mux := grpcruntime.NewServeMux()
	grpcOpts := []grpc.DialOption{grpc.WithInsecure()}
	if adminApiAddr != "" {
		err := apiserver.RegisterWorkflowAPIHandlerFromEndpoint(ctx, mux, adminApiAddr, grpcOpts)
		if err != nil {
			panic(err)
		}
	}

	if wfApiAddr != "" {
		err := apiserver.RegisterAdminAPIHandlerFromEndpoint(ctx, mux, wfApiAddr, grpcOpts)
		if err != nil {
			panic(err)
		}
	}

	if wfiApiAddr != "" {
		err := apiserver.RegisterWorkflowInvocationAPIHandlerFromEndpoint(ctx, mux, wfiApiAddr, grpcOpts)
		if err != nil {
			panic(err)
		}
	}

	gwSrv.Handler = handlers.LoggingHandler(os.Stdout, mux)
	go func() {
		err := gwSrv.ListenAndServe()
		log.WithField("err", err).Info("HTTP Gateway exited")
	}()

	log.Info("Serving HTTP API gateway at: ", gwSrv.Addr)
}

func runFissionEnvironmentProxy(proxySrv *http.Server, es fes.EventStore, wfiCache fes.CacheReader,
	wfCache fes.CacheReader, resolvers map[string]fnenv.Resolver) {

	workflowParser := parse.NewResolver(resolvers)
	workflowValidator := parse.NewValidator()
	workflowApi := workflow.NewApi(es, workflowParser)
	wfServer := apiserver.NewGrpcWorkflowApiServer(workflowApi, workflowValidator, wfCache)
	wfiApi := invocation.NewApi(es)
	wfiServer := apiserver.NewGrpcInvocationApiServer(wfiApi, wfiCache)
	proxyMux := http.NewServeMux()
	fissionProxyServer := fission.NewFissionProxyServer(wfiServer, wfServer)
	fissionProxyServer.RegisterServer(proxyMux)

	proxySrv.Handler = handlers.LoggingHandler(os.Stdout, proxyMux)
	go proxySrv.ListenAndServe()
	log.Info("Serving HTTP Fission Proxy at: ", proxySrv.Addr)
}

func setupInvocationController(invocationCache fes.CacheReader, wfCache fes.CacheReader, es fes.EventStore,
	fnRuntimes map[string]fnenv.Runtime) *controller.InvocationController {
	functionApi := function.NewApi(fnRuntimes, es)
	invocationApi := invocation.NewApi(es)
	s := &scheduler.WorkflowScheduler{}
	ep := expr.NewJavascriptExpressionParser(typedvalues.DefaultParserFormatter)
	return controller.NewInvocationController(invocationCache, wfCache, s, functionApi, invocationApi, ep)
}

func setupWorkflowController(wfCache fes.CacheReader, es fes.EventStore, fnResolvers map[string]fnenv.Resolver) *controller.WorkflowController {
	workflowApi := workflow.NewApi(es, parse.NewResolver(fnResolvers))
	return controller.NewWorkflowController(wfCache, workflowApi)
}

func runController(ctx context.Context, ctrls ...controller.Controller) {
	ctrl := controller.NewMetaController(ctrls...)
	go ctrl.Run(ctx)
}
