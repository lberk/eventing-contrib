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
	"log"
	"os"

	kafkainformer "github.com/knative/eventing-contrib/contrib/kafka/pkg/client/informers/externalversions"
	"github.com/knative/eventing-contrib/contrib/kafka/pkg/reconciler"
	eventtypeinformer "github.com/knative/eventing-contrib/pkg/client/injection/informers/eventing/v1alpha1/eventtype"
	kncontroller "github.com/knative/pkg/controller"
	"github.com/knative/pkg/logging"
	"github.com/knative/pkg/logging/logkey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	kubeinformers "k8s.io/client-go/informers"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"knative.dev/eventing-contrib/kafka/source/pkg/apis"
	controller "knative.dev/eventing-contrib/kafka/source/pkg/reconciler"
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
