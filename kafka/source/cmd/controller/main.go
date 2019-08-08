/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"log"
	"os"

	kafkainformer "github.com/knative/eventing-contrib/kafka/source/pkg/client/injection/informers/sources/v1alpha1/kafkasource"
	reconciler "github.com/knative/eventing-contrib/kafka/source/pkg/reconciler"
	"github.com/knative/eventing/pkg/duck"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	kubeinformers "k8s.io/client-go/informers"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/configmap"
	kncontroller "knative.dev/pkg/controller"
	deploymentinformer "knative.dev/pkg/injection/informers/kubeinformers/appsv1/deployment"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/logging/logkey"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	component = "kafka-controller"
)

func main() {
	// Get a config to talk to the API server
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := logCfg.Build()
	logger = logger.With(zap.String(logkey.ControllerType, "kafka-controller"))
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Setup new signalhandler
	stopCh := signals.SetupSignalHandler()

	opt := reconciler.NewOptionsOrDie(cfg, logger, stopCh)
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(opt.KubeClientSet, opt.ResyncPeriod)
	kafkaInformerFactory := kafkainformer.NewSharedInformerFactory(opt.EventingClientSet, opt.ResyncPeriod)

	kafkaInformer := kafkaInformerFactory.Sources().V1alpha1().KafkaSources()
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	eventTypeInformer := kafkaInformerFactory.Sources().V1alpha1().EventTypes()

	// Create a new Cmd to provide shared dependencies and start components

	controller := kncontroller.Impl{
		kafka.NewController(
			opt,
			kafkaInformer,
			deploymentInformer,
		),
	}

	// Watch the logging config map and dynamically update logging levels.
	// par 1 should panic if not set
	cm := os.Getenv("CONFIG_LOGGING_NAME")
	opt.ConfigMapWatcher.Watch(cm, logging.UpdateLevelFromConfigMap(logger.Sugar(), zapcore.InfoLevel, "kafka-controller"))
	// Watch the observability config map and dynamically update metrics exporter.
	opt.ConfigMapWatcher.Watch(metrics.ObservabilityConfigName, metrics.UpdateExporterFromConfigMap(component, logger))
	if err := opt.ConfigMapWatcher.Start(stopCh); err != nil {
		logger.Fatal("failed to start configuration manager", zap.Error(err))
	}

	logger.Info("Starting informers.")
	if err := kncontroller.StartInformers(
		stopCh,
		kafkaInformer.Informer(),
		deploymentInformer.Informer(),
	); err != nil {
		logger.Fatal("Failed to start informers: %v", err)
	}
	kncontroller.StartAll(stopCh, controller)

	// XXX sharedmain.Main(component, kafka.NewController)
	//	mgr, err := manager.New(cfg, manager.Options{})
	//	if err != nil {
	//		log.Fatal(err)
	//	}

	//	log.Printf("Registering Components.")

	// Setup Scheme for all resources
	//	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
	//		log.Fatal(err)
	//	}

	//	log.Printf("Setting up Controller.")

	// Setup Kafka Controller
	//	if err := controller.Add(mgr, logger.Sugar()); err != nil {
	//		log.Fatal(err)
	//	}

	//	log.Printf("Starting Apache Kafka controller.")

	// Start the Cmd
	//	log.Fatal(mgr.Start(stopCh))
}

//Adapt this as needed?
type envConfig struct {
	Image string `envconfig:"KAFKA_RA_IMAGE" required:"true"`
}

func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *kncontroller.Impl {

	kafkaInformer := kafkainformer.Get(ctx)
	deploymentInformer := deploymentinformer.Get(ctx)

	r := &Reconciler{
		Base:             reconciler.NewBase(ctx, component, cmw),
		kafkaLister:      kafkaInformer.Lister(),
		deploymentLister: deploymentInformer.Lister(),
	}
	impl := kncontroller.NewImpl(r, r.Logger, "Kafka")
	r.sinkReconciler = duck.NewInjectionSinkReconciler(ctx, impl.EnqueueKey)

	r.Logger.Info("Setting up event handlers")
	kafkaInformer.Informer().AddEventHandler(kncontroller.HandleAll(impl.Enqueue))

	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{})

	return impl
}
