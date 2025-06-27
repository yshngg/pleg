package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/homedir"
)

// id: namespace/name
type id string

type podPhaseCache map[id]corev1.PodPhase

func newPodPhaseCache() *podPhaseCache {
	return &podPhaseCache{}
}

func (p *podPhaseCache) update(namespace, name string, phase corev1.PodPhase) {
	(*p)[id(namespace+"/"+name)] = phase
}

func (p *podPhaseCache) changed(namespace, name string, phase corev1.PodPhase) bool {
	return (*p)[id(namespace+"/"+name)] != phase
}

func (p *podPhaseCache) get(namespace, name string) (corev1.PodPhase, bool) {
	phase, ok := (*p)[id(namespace+"/"+name)]
	return phase, ok
}

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

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cache := newPodPhaseCache()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme, v1.EventSource{})
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: clientset.CoreV1().Events("")})
	for event := range watcher.ResultChan() {
		if event.Type != watch.Modified {
			continue
		}
		pod := (event.Object).(*corev1.Pod)

		if !cache.changed(pod.Namespace, pod.Name, pod.Status.Phase) {
			continue
		}

		log.Printf("%v %v: %v/%v", event.Type, pod.Name, pod.Namespace, pod.Status.Phase)

		eventMessage := fmt.Sprintf("Pod phase changed to %s", pod.Status.Phase)
		oldPhase, ok := cache.get(pod.Namespace, pod.Name)
		if ok {
			eventMessage = fmt.Sprintf("Pod phase changed from %s to %s", oldPhase, pod.Status.Phase)
		}
		recorder.Event(pod, corev1.EventTypeNormal, string(pod.Status.Phase), eventMessage)

		cache.update(pod.Namespace, pod.Name, pod.Status.Phase)
	}
}
