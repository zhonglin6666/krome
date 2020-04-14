/*


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
	"errors"
	"fmt"
	"os"
	"runtime"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	kubemetrics "github.com/operator-framework/operator-sdk/pkg/kube-metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/metrics"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"krome.io/krome/pkg/apis"
	"krome.io/krome/pkg/controller"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost               = "0.0.0.0"
	metricsPort         int32 = 8383
	operatorMetricsPort int32 = 8686

	version = "1.0.0"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	app := cli.App{
		Name:            "krome",
		Usage:           "",
		Version:         version,
		CommandNotFound: cmdNotFound,
		Before: func(ctx *cli.Context) error {
			if ctx.Bool("debug") {
				logrus.SetLevel(logrus.DebugLevel)
			}
			return nil
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug, d",
				Usage: "enable debug log level",
			},
			&cli.StringFlag{
				Name:  "master, m",
				Usage: "kubernetes master",
				Value: "",
			},
			&cli.StringFlag{
				Name:  "kubeconfig, k",
				Usage: "kubernetes config",
				Value: "",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "daemon",
				Flags: []cli.Flag{},
				Action: func(c *cli.Context) error {
					if err := startDaemon(c); err != nil {
						logrus.Infof("Failed to start daemon")
					}
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatalf("Run error: %v", err)
	}
}

func startDaemon(c *cli.Context) error {
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Errorf("Failed to get watch namespace, error: %v", err)
		return err
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		logrus.Errorf("Get config error: %v", err)
		return err
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "krome-lock")
	if err != nil {
		logrus.Errorf("Leader become error: %v", err)
		return err
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace:          namespace,
		MetricsBindAddress: fmt.Sprintf("%s:%d", metricsHost, metricsPort),
	})
	if err != nil {
		logrus.Errorf("Create manager error: %v", err)
		return err
	}

	logrus.Infof("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		logrus.Errorf("Setup scheme error: %v", err)
		return err
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		logrus.Errorf("Setup controllers error: %v", err)
		return err
	}

	// Add the Metrics Service
	addMetrics(ctx, cfg, namespace)

	logrus.Infof("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logrus.Errorf("Start manager error: %v", err)
		return err
	}
	return nil
}

func cmdNotFound(ctx *cli.Context, cmd string) {
	panic(fmt.Errorf("invalid command: %s", cmd))
}

// addMetrics will create the Services and Service Monitors to allow the operator export the metrics by using
// the Prometheus operator
func addMetrics(ctx context.Context, cfg *rest.Config, namespace string) {
	if err := serveCRMetrics(cfg); err != nil {
		if errors.Is(err, k8sutil.ErrRunLocal) {
			logrus.Infof("Skipping CR metrics server creation; not running in a cluster.")
			return
		}
		logrus.Infof("Could not generate and serve custom resource metrics, error: %v", err)
	}

	// Add to the below struct any other metrics ports you want to expose.
	servicePorts := []v1.ServicePort{
		{Port: metricsPort, Name: metrics.OperatorPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: metricsPort}},
		{Port: operatorMetricsPort, Name: metrics.CRPortName, Protocol: v1.ProtocolTCP, TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: operatorMetricsPort}},
	}

	// Create Service object to expose the metrics port(s).
	service, err := metrics.CreateMetricsService(ctx, cfg, servicePorts)
	if err != nil {
		logrus.Infof("Could not create metrics Service error: %v", err)
	}

	// CreateServiceMonitors will automatically create the prometheus-operator ServiceMonitor resources
	// necessary to configure Prometheus to scrape metrics from this operator.
	services := []*v1.Service{service}
	_, err = metrics.CreateServiceMonitors(cfg, namespace, services)
	if err != nil {
		logrus.Infof("Could not create ServiceMonitor object error: %v", err)
		// If this operator is deployed to a cluster without the prometheus-operator running, it will return
		// ErrServiceMonitorNotPresent, which can be used to safely skip ServiceMonitor creation.
		if err == metrics.ErrServiceMonitorNotPresent {
			logrus.Infof("Install prometheus-operator in your cluster to create ServiceMonitor objects error: %v", err)
		}
	}
}

// serveCRMetrics gets the Operator/CustomResource GVKs and generates metrics based on those types.
// It serves those metrics on "http://metricsHost:operatorMetricsPort".
func serveCRMetrics(cfg *rest.Config) error {
	// Below function returns filtered operator/CustomResource specific GVKs.
	// For more control override the below GVK list with your own custom logic.
	filteredGVK, err := k8sutil.GetGVKsFromAddToScheme(apis.AddToScheme)
	if err != nil {
		return err
	}
	// Get the namespace the operator is currently deployed in.
	operatorNs, err := k8sutil.GetOperatorNamespace()
	if err != nil {
		return err
	}
	// To generate metrics in other namespaces, add the values below.
	ns := []string{operatorNs}
	// Generate and serve custom resource specific metrics.
	err = kubemetrics.GenerateAndServeCRMetrics(cfg, ns, filteredGVK, metricsHost, operatorMetricsPort)
	if err != nil {
		return err
	}
	return nil
}
