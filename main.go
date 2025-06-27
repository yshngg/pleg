package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// TODO: add flags
// - namespace
// - kubeconfig

// initConfig initializes Kubernetes configuration by first attempting in-cluster config,
// then falling back to kubeconfig file if running outside cluster.
// Returns the loaded configuration and error if any.
func initConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err == nil || !errors.Is(err, rest.ErrNotInCluster) {
		return config, err
	}

	var kubeconfig string
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func main() {
	// Initialize Kubernetes configuration
	config, err := initConfig()
	if err != nil {
		log.Fatalf("initialize Kubernetes configuration, err: %v", err)
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("create clientset, err: %v", err)
	}

	watcher, err := clientset.CoreV1().Pods("").Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("watch pods, err: %v", err)
	}
	defer watcher.Stop()
	for event := range watcher.ResultChan() {
		pod := (event.Object).(*corev1.Pod)
		log.Printf("%v %v: %v/%v", event.Type, pod.Name, pod.Namespace, pod.Status.Phase)
	}
}
