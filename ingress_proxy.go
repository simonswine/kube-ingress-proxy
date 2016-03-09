package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	kube "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util"
)

type IngressProxy struct {
	IngressName      string
	IngressNamespace string
	Ingress          *extensions.Ingress
	HttpPort         int16
	HttpsPort        int16
	kubeClient       *kube.Client
	ingClient        kube.IngressInterface
	backends         map[string]*httputil.ReverseProxy
	backendsLock     sync.RWMutex
	daemonWaitGroup  sync.WaitGroup
}

func NewIngressProxy() *IngressProxy {
	i := &IngressProxy{
		HttpPort:  8080,
		HttpsPort: 8443,
	}
	i.backends = make(map[string]*httputil.ReverseProxy)
	return i
}

func (ip *IngressProxy) urlFromBackend(b *extensions.IngressBackend) *url.URL {
	return &url.URL{
		Host: fmt.Sprintf(
			"%s.%s.svc.cluster.local:%d",
			b.ServiceName,
			ip.Ingress.ObjectMeta.Namespace,
			b.ServicePort.IntVal,
		),
		Scheme: "http",
	}
}

func (ip *IngressProxy) routeRequest(r *http.Request) *httputil.ReverseProxy {

	backend := ip.routeRequestToBackend(r)
	if backend == nil {
		return nil
	}

	backendKey := fmt.Sprintf("%s:%s", backend.ServiceName, backend.ServicePort)

	if backendProxy, ok := ip.backends[backendKey]; ok {
		return backendProxy
	}

	backendProxy := httputil.NewSingleHostReverseProxy(
		ip.urlFromBackend(backend),
	)
	ip.backends[backendKey] = backendProxy

	return backendProxy
}

func (ip *IngressProxy) routeRequestToBackend(r *http.Request) *extensions.IngressBackend {

	for _, rule := range ip.Ingress.Spec.Rules {
		if strings.ToLower(rule.Host) != strings.ToLower(r.Host) {
			//skip if hostname does not match
			continue
		}

		matchingLen := 0
		matchingBackend := -1

		for pos, path := range rule.HTTP.Paths {
			fullPath := r.URL.Path
			prefixPath := path.Path
			if prefixPath == "" {
				prefixPath = "/"
			}
			if strings.HasPrefix(fullPath, prefixPath) && len(prefixPath) > matchingLen {
				matchingLen = len(path.Path)
				matchingBackend = pos
			}
		}

		if matchingBackend != -1 {
			return &rule.HTTP.Paths[matchingBackend].Backend
		}

	}

	return ip.Ingress.Spec.Backend
}

func (ip *IngressProxy) httpError(w http.ResponseWriter, msg string, code int) {
	http.Error(w, msg, code)
	log.Warnf("code=%d msg=%s", code, msg)
}

func (ip *IngressProxy) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-KubeIngressProxy", "go alter!")
	log.Infof("host=%s path=%s method=%s", r.Host, r.URL.Path, r.Method)

	proxy := ip.routeRequest(r)
	if proxy == nil {
		ip.httpError(w, "No backend found", 503)
		return
	}
	proxy.ServeHTTP(w, r)
}

func (ip *IngressProxy) getKubeClient() (*kube.Client, error) {
	return kube.NewInCluster()
}

func (ip *IngressProxy) getConfig() {

}

func (ip *IngressProxy) readEnv() error {

	ip.IngressName = os.Getenv("INGRESS_NAME")
	if len(ip.IngressName) == 0 {
		return errors.New("Please provide an ingress resource name in env var INGRESS_NAME")
	}

	ip.IngressNamespace = os.Getenv("INGRESS_NAMESPACE")
	if len(ip.IngressNamespace) == 0 {
		ip.IngressNamespace = api.NamespaceDefault
	}

	return nil
}

func (ip *IngressProxy) Init() error {

	err := ip.readEnv()
	if err != nil {
		return err
	}

	kubeClient, err := kube.NewInCluster()
	if err != nil {
		return err
	}
	ip.kubeClient = kubeClient

	ip.ingClient = ip.kubeClient.Extensions().Ingress(ip.IngressNamespace)

	ingress, err := ip.ingClient.Get(ip.IngressName)
	if err != nil {
		return err
	}
	ip.SetIngress(ingress)

	return nil
}

func (ip *IngressProxy) SetIngress(ing *extensions.Ingress) {
	// TODO: Use read write lock for that
	ip.Ingress = ing
}

func (ip *IngressProxy) WatchConfig() {

	rateLimiter := util.NewTokenBucketRateLimiter(0.1, 1)

	for {
		rateLimiter.Accept()

		ingress, err := ip.ingClient.Get(ip.IngressName)
		if err != nil {
			log.Warnf("Getting config failed: %s", err)
		}

		if reflect.DeepEqual(ip.Ingress, ingress) {
			continue
		}

		log.Infof("Upgrade ingress config")
		ip.SetIngress(ingress)
	}

}

func (ip *IngressProxy) Start() {

	http.HandleFunc("/", ip.handle)


	// http server port
	ip.daemonWaitGroup.Add(1)
	go func() {
		defer ip.daemonWaitGroup.Done()
		log.Infof("Start listening for HTTP on port %d", ip.HttpPort)
		err := http.ListenAndServe(fmt.Sprintf(":%d", ip.HttpPort), nil)
		log.Error(err)
	}()

	// https server port
	ip.daemonWaitGroup.Add(1)
	go func() {
		defer ip.daemonWaitGroup.Done()
		// getting secrets tls
		secretClient := ip.kubeClient.Secrets(ip.IngressNamespace)
		secretName := ip.Ingress.Spec.TLS[0].SecretName
		secret, err := secretClient.Get(secretName)
		if err != nil {
			log.Errorf("TLS secret '%s' not found: %s", secretName, err)
			return
		}

		// hacky approach TODO: do this in memory
		dir, err := ioutil.TempDir("", "example")
		if err != nil {
			log.Error(err)
			return
		}

		defer os.RemoveAll(dir) // clean up

		certPath := filepath.Join(dir, "cert.pem")
		keyPath := filepath.Join(dir, "key.pem")

		if err := ioutil.WriteFile(certPath, secret.Data[api.TLSCertKey], 0644); err != nil {
			log.Error(err)
			return
		}
		if err := ioutil.WriteFile(keyPath, secret.Data[api.TLSPrivateKeyKey], 0600); err != nil {
			log.Error(err)
			return
		}
		log.Infof("Start listening for HTTPS on port %d", ip.HttpsPort)
		err = http.ListenAndServeTLS(fmt.Sprintf(":%d", ip.HttpsPort), certPath, keyPath, nil)
		log.Error(err)

		http.ListenAndServe(fmt.Sprintf(":%d", ip.HttpPort), nil)
	}()

	// config watcher
	ip.daemonWaitGroup.Add(1)
	go func() {
		defer ip.daemonWaitGroup.Done()
		ip.WatchConfig()
	}()

	ip.daemonWaitGroup.Wait()
}
